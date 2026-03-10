package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestHTTPClientSendStreamReturnsRecoverableErrorAfterAccepted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: accepted\n")
		accepted, _ := json.Marshal(api.ChatStreamEvent{
			Type:                  api.ChatStreamEventAccepted,
			EventID:               "evt_1",
			Sequence:              1,
			StreamProtocolVersion: api.ChatStreamProtocolVersion,
		})
		fmt.Fprintf(w, "data: %s\n\n", accepted)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "", 3*time.Second, 10*time.Millisecond, 100*time.Millisecond)
	_, err := client.SendStream(context.Background(), api.IngestRequest{
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
	_, err := client.SendStream(context.Background(), api.IngestRequest{
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

func TestHTTPClientSendStreamTimesOutWaitingForHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: accepted\n")
		accepted, _ := json.Marshal(api.ChatStreamEvent{
			Type:                  api.ChatStreamEventAccepted,
			EventID:               "evt_2",
			Sequence:              1,
			StreamProtocolVersion: api.ChatStreamProtocolVersion,
		})
		fmt.Fprintf(w, "data: %s\n\n", accepted)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "", 50*time.Millisecond, 10*time.Millisecond, 100*time.Millisecond)
	start := time.Now()
	_, err := client.SendStream(context.Background(), api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "c2", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:c2:1",
		Timestamp:      "2026-03-07T00:00:00Z",
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}, noopStreamHandler{})
	if err == nil {
		t.Fatal("expected timeout error while waiting for headers")
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("expected header timeout quickly, took %s", elapsed)
	}
}

func TestHTTPClientSendStreamTimesOutWaitingForAcceptedEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "", 50*time.Millisecond, 10*time.Millisecond, 100*time.Millisecond)
	start := time.Now()
	_, err := client.SendStream(context.Background(), api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "c3", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:c3:1",
		Timestamp:      "2026-03-07T00:00:00Z",
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}, noopStreamHandler{})
	if err == nil {
		t.Fatal("expected timeout error while waiting for accepted event")
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("expected accepted-event timeout quickly, took %s", elapsed)
	}
}

func TestHTTPClientSendStreamPollsUntilEventReallyTerminalAfterDone(t *testing.T) {
	var pollCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat:stream":
			w.Header().Set("Content-Type", "text/event-stream")
			accepted, _ := json.Marshal(api.ChatStreamEvent{
				Type:                  api.ChatStreamEventAccepted,
				EventID:               "evt_3",
				Sequence:              1,
				StreamProtocolVersion: api.ChatStreamProtocolVersion,
			})
			fmt.Fprintf(w, "event: accepted\n")
			fmt.Fprintf(w, "data: %s\n\n", accepted)
			done, _ := json.Marshal(api.ChatStreamEvent{
				Type:     api.ChatStreamEventDone,
				EventID:  "evt_3",
				Sequence: 2,
				EventRecord: &api.EventRecord{
					EventID:      "evt_3",
					Status:       model.EventStatusProcessed,
					OutboxStatus: model.OutboxStatusPending,
				},
			})
			fmt.Fprintf(w, "event: done\n")
			fmt.Fprintf(w, "data: %s\n\n", done)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case r.Method == http.MethodGet && r.URL.Path == "/v1/events/evt_3":
			attempt := pollCount.Add(1)
			rec := api.EventRecord{
				EventID:      "evt_3",
				Status:       model.EventStatusProcessed,
				OutboxStatus: model.OutboxStatusPending,
			}
			if attempt >= 2 {
				rec.OutboxStatus = model.OutboxStatusSent
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(rec); err != nil {
				t.Fatalf("encode event record: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "", 100*time.Millisecond, 10*time.Millisecond, 200*time.Millisecond)
	rec, err := client.SendStream(context.Background(), api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "c4", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:c4:1",
		Timestamp:      "2026-03-07T00:00:00Z",
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}, noopStreamHandler{})
	if err != nil {
		t.Fatalf("SendStream returned error: %v", err)
	}
	if rec.OutboxStatus != model.OutboxStatusSent {
		t.Fatalf("expected sent outbox status after polling, got %+v", rec)
	}
	if pollCount.Load() < 2 {
		t.Fatalf("expected at least 2 polls, got %d", pollCount.Load())
	}
}

type noopStreamHandler struct{}

func (noopStreamHandler) HandleStreamEvent(api.ChatStreamEvent) error { return nil }
