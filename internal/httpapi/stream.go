package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const streamKeepaliveInterval = 15 * time.Second

func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	var req model.IngestRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "invalid json"})
		return
	}

	sub := s.streamHub.Reserve(req.IdempotencyKey)
	defer s.streamHub.Release(sub)

	accepted, apiErr := s.gateway.Accept(r.Context(), req)
	if apiErr != nil {
		writeAPIError(w, apiErr)
		return
	}
	terminal := s.streamHub.Attach(sub, accepted.Result.EventID)

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAPIError(w, &gateway.APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: "streaming unsupported"})
		return
	}
	initSSEHeaders(w)
	acceptedEvent := model.ChatStreamEvent{
		Type:                  model.ChatStreamEventAccepted,
		EventID:               accepted.Result.EventID,
		Sequence:              1,
		At:                    time.Now().UTC(),
		StreamProtocolVersion: model.ChatStreamProtocolVersion,
		IngestResponse:        &accepted.Response,
	}
	if err := writeSSEEvent(w, flusher, acceptedEvent); err != nil {
		return
	}
	if terminal != nil {
		_ = writeSSEEvent(w, flusher, *terminal)
		return
	}
	if eventRec, ok, err := s.db.GetEvent(r.Context(), accepted.Result.EventID); err == nil && ok {
		if terminalEvent := terminalEventFromRecord(eventRec); terminalEvent != nil {
			s.streamHub.PublishTerminal(accepted.Result.EventID, *terminalEvent)
		}
	}

	for {
		waitCtx, cancel := context.WithTimeout(r.Context(), streamKeepaliveInterval)
		event, ok := sub.Next(waitCtx)
		waitErr := waitCtx.Err()
		cancel()
		if ok {
			if err := writeSSEEvent(w, flusher, event); err != nil {
				return
			}
			if event.IsTerminal() {
				return
			}
			continue
		}
		if r.Context().Err() != nil {
			return
		}
		if waitErr == context.DeadlineExceeded {
			if err := writeSSEComment(w, flusher, "keepalive"); err != nil {
				return
			}
			continue
		}
		return
	}
}

func initSSEHeaders(w http.ResponseWriter) {
	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")
	headers.Set("X-Accel-Buffering", "no")
}

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, event model.ChatStreamEvent) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeSSEComment(w http.ResponseWriter, flusher http.Flusher, comment string) error {
	if _, err := fmt.Fprintf(w, ": %s\n\n", comment); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func terminalEventFromRecord(rec model.EventRecord) *model.ChatStreamEvent {
	switch rec.Status {
	case model.EventStatusFailed:
		return &model.ChatStreamEvent{
			Type:        model.ChatStreamEventError,
			EventID:     rec.EventID,
			At:          rec.UpdatedAt,
			EventRecord: &rec,
			Error:       rec.Error,
		}
	case model.EventStatusProcessed, model.EventStatusSuppressed:
		return &model.ChatStreamEvent{
			Type:        model.ChatStreamEventDone,
			EventID:     rec.EventID,
			At:          rec.UpdatedAt,
			EventRecord: &rec,
		}
	default:
		return nil
	}
}

func readSSEEvent(r *bufio.Reader) (string, []byte, error) {
	var (
		eventType string
		data      []byte
	)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", nil, err
		}
		if line == "\n" {
			if eventType == "" && len(data) == 0 {
				continue
			}
			return eventType, data, nil
		}
		if len(line) > 0 && line[0] == ':' {
			continue
		}
		switch {
		case len(line) > len("event: ") && line[:len("event: ")] == "event: ":
			eventType = trimSSELine(line[len("event: "):])
		case len(line) > len("data: ") && line[:len("data: ")] == "data: ":
			if data == nil {
				data = []byte(trimSSELine(line[len("data: "):]))
				continue
			}
			data = append(data, '\n')
			data = append(data, trimSSELine(line[len("data: "):])...)
		}
	}
}

func trimSSELine(in string) string {
	for len(in) > 0 && (in[len(in)-1] == '\n' || in[len(in)-1] == '\r') {
		in = in[:len(in)-1]
	}
	return in
}
