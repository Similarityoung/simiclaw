package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
	httpingest "github.com/similarityyoung/simiclaw/internal/http/ingest"
	httpquery "github.com/similarityyoung/simiclaw/internal/http/query"
	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
	runtimeevents "github.com/similarityyoung/simiclaw/internal/runtime/events"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const streamKeepaliveInterval = 15 * time.Second

type Gateway interface {
	Accept(ctx context.Context, in gatewaymodel.NormalizedIngress) (gateway.AcceptedIngest, *gateway.APIError)
}

type Query interface {
	GetEvent(ctx context.Context, eventID string) (querymodel.EventRecord, bool, error)
}

type Handlers struct {
	gateway Gateway
	query   Query
	hub     *runtimeevents.Hub
	logger  *logging.Logger
}

func NewHandlers(gateway Gateway, query Query, hub *runtimeevents.Hub) *Handlers {
	return &Handlers{
		gateway: gateway,
		query:   query,
		hub:     hub,
		logger:  logging.L("http.stream"),
	}
}

func (h *Handlers) HandleChatStream(w http.ResponseWriter, r *http.Request) {
	var req api.IngestRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("stream decode failed",
			logging.String("method", r.Method),
			logging.String("path", r.URL.Path),
			logging.String("error_code", model.ErrorCodeInvalidArgument),
			logging.Int("status_code", http.StatusBadRequest),
			logging.Error(err),
		)
		httpingest.WriteAPIError(w, &gateway.APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "invalid json"})
		return
	}
	normalized, apiErr := httpingest.NormalizeAPIRequest(req)
	if apiErr != nil {
		h.logger.Warn("stream normalize failed",
			logging.String("method", r.Method),
			logging.String("path", r.URL.Path),
			logging.String("error_code", apiErr.Code),
			logging.Int("status_code", apiErr.StatusCode),
			logging.String("message", apiErr.Message),
		)
		httpingest.WriteAPIError(w, apiErr)
		return
	}

	sub := h.hub.Reserve(req.IdempotencyKey)
	defer h.hub.Release(sub)

	accepted, apiErr := h.gateway.Accept(r.Context(), normalized)
	if apiErr != nil {
		httpingest.WriteAPIError(w, apiErr)
		return
	}
	replay := h.hub.Attach(sub, accepted.Result.EventID)

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.logger.Error("streaming unsupported",
			logging.String("method", r.Method),
			logging.String("path", r.URL.Path),
			logging.String("event_id", accepted.Result.EventID),
			logging.String("session_key", accepted.Result.SessionKey),
			logging.String("session_id", accepted.Result.SessionID),
			logging.String("error_code", model.ErrorCodeInternal),
			logging.Int("status_code", http.StatusInternalServerError),
		)
		httpingest.WriteAPIError(w, &gateway.APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: "streaming unsupported"})
		return
	}
	h.logger.Info("stream attached",
		logging.String("event_id", accepted.Result.EventID),
		logging.String("session_key", accepted.Result.SessionKey),
		logging.String("session_id", accepted.Result.SessionID),
	)
	initSSEHeaders(w)
	acceptedEvent := api.ChatStreamEvent{
		Type:                  api.ChatStreamEventAccepted,
		EventID:               accepted.Result.EventID,
		Sequence:              1,
		At:                    time.Now().UTC(),
		StreamProtocolVersion: api.ChatStreamProtocolVersion,
		IngestResponse:        &accepted.Response,
	}
	if err := writeSSEEvent(w, flusher, acceptedEvent); err != nil {
		return
	}
	for _, event := range replay {
		apiEvent := runtimeEventToAPI(event)
		if err := writeSSEEvent(w, flusher, apiEvent); err != nil {
			return
		}
		if apiEvent.IsTerminal() {
			return
		}
	}
	if eventRec, ok, err := h.query.GetEvent(r.Context(), accepted.Result.EventID); err == nil && ok {
		if terminalEvent := terminalRuntimeEventFromRecord(eventRec); terminalEvent != nil {
			_ = writeSSEEvent(w, flusher, runtimeEventToAPI(h.hub.PublishTerminal(*terminalEvent)))
			return
		}
	}

	for {
		waitCtx, cancel := context.WithTimeout(r.Context(), streamKeepaliveInterval)
		event, ok := sub.Next(waitCtx)
		waitErr := waitCtx.Err()
		cancel()
		if ok {
			apiEvent := runtimeEventToAPI(event)
			if err := writeSSEEvent(w, flusher, apiEvent); err != nil {
				return
			}
			if apiEvent.IsTerminal() {
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

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, event api.ChatStreamEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", body); err != nil {
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

func terminalRuntimeEventFromRecord(rec querymodel.EventRecord) *runtimemodel.RuntimeEvent {
	eventRecord := queryEventRecordToRuntime(rec)
	switch rec.Status {
	case model.EventStatusFailed:
		return &runtimemodel.RuntimeEvent{
			Kind:        runtimemodel.RuntimeEventFailed,
			EventID:     rec.EventID,
			RunID:       rec.RunID,
			SessionKey:  rec.SessionKey,
			SessionID:   rec.SessionID,
			OccurredAt:  rec.UpdatedAt,
			Error:       rec.Error,
			EventRecord: &eventRecord,
		}
	case model.EventStatusProcessed, model.EventStatusSuppressed:
		return &runtimemodel.RuntimeEvent{
			Kind:        runtimemodel.RuntimeEventCompleted,
			EventID:     rec.EventID,
			RunID:       rec.RunID,
			SessionKey:  rec.SessionKey,
			SessionID:   rec.SessionID,
			OccurredAt:  rec.UpdatedAt,
			EventRecord: &eventRecord,
		}
	default:
		return nil
	}
}

func runtimeEventToAPI(event runtimemodel.RuntimeEvent) api.ChatStreamEvent {
	out := api.ChatStreamEvent{
		EventID:  event.EventID,
		Sequence: event.Sequence,
		At:       nonZeroTime(event.OccurredAt),
	}
	switch event.Kind {
	case runtimemodel.RuntimeEventClaimed, runtimemodel.RuntimeEventExecuting, runtimemodel.RuntimeEventFinalizeStarted:
		out.Type = api.ChatStreamEventStatus
		out.Status = "processing"
		out.Message = event.Message
	case runtimemodel.RuntimeEventReasoningDelta:
		out.Type = api.ChatStreamEventReasoningDelta
		out.Delta = event.Delta
	case runtimemodel.RuntimeEventTextDelta:
		out.Type = api.ChatStreamEventTextDelta
		out.Delta = event.Delta
	case runtimemodel.RuntimeEventToolStarted:
		out.Type = api.ChatStreamEventToolStart
		out.ToolCallID = event.ToolCallID
		out.ToolName = event.ToolName
		out.Args = event.Args
		out.Truncated = event.Truncated
	case runtimemodel.RuntimeEventToolFinished:
		out.Type = api.ChatStreamEventToolResult
		out.ToolCallID = event.ToolCallID
		out.ToolName = event.ToolName
		out.Result = event.Result
		out.Truncated = event.Truncated
		out.Error = event.Error
	case runtimemodel.RuntimeEventFailed:
		out.Type = api.ChatStreamEventError
		out.Error = event.Error
		if event.EventRecord != nil {
			apiRec := runtimeEventRecordToAPI(*event.EventRecord)
			out.EventRecord = &apiRec
			if out.Error == nil {
				out.Error = apiRec.Error
			}
		}
	case runtimemodel.RuntimeEventCompleted:
		out.Type = api.ChatStreamEventDone
		if event.EventRecord != nil {
			apiRec := runtimeEventRecordToAPI(*event.EventRecord)
			out.EventRecord = &apiRec
		}
	}
	return out
}

func queryEventRecordToRuntime(rec querymodel.EventRecord) runtimemodel.EventRecord {
	return runtimemodel.EventRecord{
		EventID:           rec.EventID,
		Status:            rec.Status,
		OutboxStatus:      rec.OutboxStatus,
		SessionKey:        rec.SessionKey,
		SessionID:         rec.SessionID,
		RunID:             rec.RunID,
		RunMode:           rec.RunMode,
		AssistantReply:    rec.AssistantReply,
		OutboxID:          rec.OutboxID,
		ProcessingLease:   rec.ProcessingLease,
		ReceivedAt:        rec.ReceivedAt,
		CreatedAt:         rec.CreatedAt,
		UpdatedAt:         rec.UpdatedAt,
		PayloadHash:       rec.PayloadHash,
		Provider:          rec.Provider,
		Model:             rec.Model,
		ProviderRequestID: rec.ProviderRequestID,
		Error:             rec.Error,
	}
}

func runtimeEventRecordToAPI(rec runtimemodel.EventRecord) api.EventRecord {
	return httpquery.ToAPIEventRecord(querymodel.EventRecord{
		EventID:           rec.EventID,
		Status:            rec.Status,
		OutboxStatus:      rec.OutboxStatus,
		SessionKey:        rec.SessionKey,
		SessionID:         rec.SessionID,
		RunID:             rec.RunID,
		RunMode:           rec.RunMode,
		AssistantReply:    rec.AssistantReply,
		OutboxID:          rec.OutboxID,
		ProcessingLease:   rec.ProcessingLease,
		ReceivedAt:        rec.ReceivedAt,
		CreatedAt:         rec.CreatedAt,
		UpdatedAt:         rec.UpdatedAt,
		PayloadHash:       rec.PayloadHash,
		Provider:          rec.Provider,
		Model:             rec.Model,
		ProviderRequestID: rec.ProviderRequestID,
		Error:             rec.Error,
	})
}

func nonZeroTime(in time.Time) time.Time {
	if in.IsZero() {
		return time.Now().UTC()
	}
	return in
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
