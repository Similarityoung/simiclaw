package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestHTTPClientSendStreamReturnsRecoverableErrorAfterAccepted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: accepted\n")
		accepted, _ := json.Marshal(model.ChatStreamEvent{
			Type:                  model.ChatStreamEventAccepted,
			EventID:               "evt_1",
			Sequence:              1,
			StreamProtocolVersion: model.ChatStreamProtocolVersion,
		})
		fmt.Fprintf(w, "data: %s\n\n", accepted)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "", 3*time.Second, 10*time.Millisecond, 100*time.Millisecond)
	_, err := client.SendStream(context.Background(), model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "c1", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:c1:1",
		Timestamp:      "2026-03-07T00:00:00Z",
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}, noopStreamHandler{})
	var recoverable *StreamRecoverableError
	if !errors.As(err, &recoverable) {
		t.Fatalf("expected recoverable error, got %v", err)
	}
	if recoverable.EventID != "evt_1" {
		t.Fatalf("unexpected recoverable event id: %+v", recoverable)
	}
}

func TestHTTPClientSendStreamReportsUnsupportedWhenNotSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "", 3*time.Second, 10*time.Millisecond, 100*time.Millisecond)
	_, err := client.SendStream(context.Background(), model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "c1", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:c1:1",
		Timestamp:      "2026-03-07T00:00:00Z",
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}, noopStreamHandler{})
	if !errors.Is(err, ErrStreamUnsupported) {
		t.Fatalf("expected ErrStreamUnsupported, got %v", err)
	}
}

type noopStreamHandler struct{}

func (noopStreamHandler) HandleStreamEvent(model.ChatStreamEvent) error { return nil }
