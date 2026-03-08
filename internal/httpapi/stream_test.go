package httpapi_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/bootstrap"
	"github.com/similarityyoung/simiclaw/internal/channels/cli"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestChatStreamAcceptedThenDone(t *testing.T) {
	app := newStreamTestApp(t)
	server := httptest.NewServer(app.Handler)
	defer server.Close()

	req := cli.BuildIngestRequest("stream-done", "u1", 1, "hello")
	resp := postStreamRequest(t, server.URL, req)
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected SSE content type, got %q", got)
	}
	reader := bufio.NewReader(resp.Body)

	accepted := readStreamEvent(t, reader)
	if accepted.Type != model.ChatStreamEventAccepted {
		t.Fatalf("expected accepted event, got %+v", accepted)
	}
	if accepted.StreamProtocolVersion != model.ChatStreamProtocolVersion {
		t.Fatalf("unexpected protocol version: %+v", accepted)
	}

	app.StreamHub.PublishTerminal(accepted.EventID, model.ChatStreamEvent{
		Type: model.ChatStreamEventDone,
		EventRecord: &model.EventRecord{
			EventID:        accepted.EventID,
			Status:         model.EventStatusProcessed,
			AssistantReply: "done",
			UpdatedAt:      time.Now().UTC(),
		},
	})
	done := readStreamEvent(t, reader)
	if done.Type != model.ChatStreamEventDone {
		t.Fatalf("expected done event, got %+v", done)
	}
	if done.EventRecord == nil || done.EventRecord.AssistantReply != "done" {
		t.Fatalf("unexpected done payload: %+v", done)
	}
}

func TestChatStreamAcceptedThenError(t *testing.T) {
	app := newStreamTestApp(t)
	server := httptest.NewServer(app.Handler)
	defer server.Close()

	req := cli.BuildIngestRequest("stream-error", "u1", 1, "hello")
	resp := postStreamRequest(t, server.URL, req)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	accepted := readStreamEvent(t, reader)
	app.StreamHub.PublishTerminal(accepted.EventID, model.ChatStreamEvent{
		Type: model.ChatStreamEventError,
		Error: &model.ErrorBlock{
			Code:    model.ErrorCodeInternal,
			Message: "boom",
		},
		EventRecord: &model.EventRecord{
			EventID:   accepted.EventID,
			Status:    model.EventStatusFailed,
			UpdatedAt: time.Now().UTC(),
			Error: &model.ErrorBlock{
				Code:    model.ErrorCodeInternal,
				Message: "boom",
			},
		},
	})
	failed := readStreamEvent(t, reader)
	if failed.Type != model.ChatStreamEventError {
		t.Fatalf("expected error event, got %+v", failed)
	}
	if failed.Error == nil || failed.Error.Message != "boom" {
		t.Fatalf("unexpected error payload: %+v", failed)
	}
}

func newStreamTestApp(t *testing.T) *bootstrap.App {
	t.Helper()
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	if err := store.InitWorkspace(cfg.Workspace, false, cfg.DBBusyTimeout.Duration); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	app, err := bootstrap.NewApp(cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(app.Stop)
	return app
}

func postStreamRequest(t *testing.T, baseURL string, req model.IngestRequest) *http.Response {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := http.Post(baseURL+"/v1/chat:stream", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post stream request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	return resp
}

func readStreamEvent(t *testing.T, reader *bufio.Reader) model.ChatStreamEvent {
	t.Helper()
	eventType, data, err := readSSEEventForTest(reader)
	if err != nil {
		t.Fatalf("read SSE event: %v", err)
	}
	var event model.ChatStreamEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("decode SSE payload: %v", err)
	}
	if string(event.Type) != eventType {
		t.Fatalf("event type mismatch: header=%s payload=%s", eventType, event.Type)
	}
	return event
}

func readSSEEventForTest(r *bufio.Reader) (string, []byte, error) {
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
			eventType = trimSSELineForTest(line[len("event: "):])
		case len(line) > len("data: ") && line[:len("data: ")] == "data: ":
			if data == nil {
				data = []byte(trimSSELineForTest(line[len("data: "):]))
				continue
			}
			data = append(data, '\n')
			data = append(data, trimSSELineForTest(line[len("data: "):])...)
		}
	}
}

func trimSSELineForTest(in string) string {
	for len(in) > 0 && (in[len(in)-1] == '\n' || in[len(in)-1] == '\r') {
		in = in[:len(in)-1]
	}
	return in
}
