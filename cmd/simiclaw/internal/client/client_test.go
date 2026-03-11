package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestStreamChatReturnsRecoverableResultAfterAcceptedDisconnect(t *testing.T) {
	var pollCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat:stream":
			w.Header().Set("Content-Type", "text/event-stream")
			accepted, _ := json.Marshal(api.ChatStreamEvent{
				Type:                  api.ChatStreamEventAccepted,
				EventID:               "evt_1",
				Sequence:              1,
				StreamProtocolVersion: api.ChatStreamProtocolVersion,
			})
			fmt.Fprintf(w, "event: accepted\n")
			fmt.Fprintf(w, "data: %s\n\n", accepted)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case r.Method == http.MethodGet && r.URL.Path == "/v1/events/evt_1":
			pollCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(api.EventRecord{
				EventID:        "evt_1",
				Status:         model.EventStatusProcessed,
				AssistantReply: "hello world",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTestClient(server.URL, 100*time.Millisecond, 10*time.Millisecond, 200*time.Millisecond)
	rec, err := client.StreamChat(context.Background(), testIngestRequest("c1"), nil)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if rec.EventID != "evt_1" || rec.AssistantReply != "hello world" {
		t.Fatalf("unexpected recovered record: %+v", rec)
	}
	if pollCount.Load() == 0 {
		t.Fatalf("expected polling fallback after accepted disconnect")
	}
}

func TestRawStreamChatReportsUnsupportedWhenNotSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := newTestClient(server.URL, 100*time.Millisecond, 10*time.Millisecond, 200*time.Millisecond)
	_, err := client.streamChat(context.Background(), testIngestRequest("c2"), nil)
	if !errors.Is(err, ErrStreamUnsupported) {
		t.Fatalf("expected ErrStreamUnsupported, got %v", err)
	}
}

func TestRawStreamChatTimesOutWaitingForHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: accepted\n")
	}))
	defer server.Close()

	client := newTestClient(server.URL, 50*time.Millisecond, 10*time.Millisecond, 100*time.Millisecond)
	start := time.Now()
	_, err := client.streamChat(context.Background(), testIngestRequest("c3"), nil)
	if err == nil {
		t.Fatal("expected timeout while waiting for headers")
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("expected fast header timeout, took %s", elapsed)
	}
}

func TestRawStreamChatTimesOutWaitingForAcceptedEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	client := newTestClient(server.URL, 50*time.Millisecond, 10*time.Millisecond, 100*time.Millisecond)
	start := time.Now()
	_, err := client.streamChat(context.Background(), testIngestRequest("c4"), nil)
	if err == nil {
		t.Fatal("expected timeout while waiting for accepted event")
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("expected fast accepted timeout, took %s", elapsed)
	}
}

func TestStreamChatPollsUntilEventReallyTerminalAfterDone(t *testing.T) {
	var pollCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat:stream":
			w.Header().Set("Content-Type", "text/event-stream")
			accepted, _ := json.Marshal(api.ChatStreamEvent{
				Type:                  api.ChatStreamEventAccepted,
				EventID:               "evt_5",
				Sequence:              1,
				StreamProtocolVersion: api.ChatStreamProtocolVersion,
			})
			done, _ := json.Marshal(api.ChatStreamEvent{
				Type:     api.ChatStreamEventDone,
				EventID:  "evt_5",
				Sequence: 2,
				EventRecord: &api.EventRecord{
					EventID:      "evt_5",
					Status:       model.EventStatusProcessed,
					OutboxStatus: model.OutboxStatusPending,
				},
			})
			fmt.Fprintf(w, "event: accepted\n")
			fmt.Fprintf(w, "data: %s\n\n", accepted)
			fmt.Fprintf(w, "event: done\n")
			fmt.Fprintf(w, "data: %s\n\n", done)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case r.Method == http.MethodGet && r.URL.Path == "/v1/events/evt_5":
			attempt := pollCount.Add(1)
			rec := api.EventRecord{
				EventID:      "evt_5",
				Status:       model.EventStatusProcessed,
				OutboxStatus: model.OutboxStatusPending,
			}
			if attempt >= 2 {
				rec.OutboxStatus = model.OutboxStatusSent
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(rec)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTestClient(server.URL, 100*time.Millisecond, 10*time.Millisecond, 200*time.Millisecond)
	rec, err := client.StreamChat(context.Background(), testIngestRequest("c5"), nil)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if rec.OutboxStatus != model.OutboxStatusSent {
		t.Fatalf("expected sent outbox status after polling, got %+v", rec)
	}
	if pollCount.Load() < 2 {
		t.Fatalf("expected at least 2 polls, got %d", pollCount.Load())
	}
}

func newTestClient(baseURL string, requestTimeout, pollInterval, pollTimeout time.Duration) *Client {
	client := New(baseURL, "", requestTimeout)
	client.requestTimeout = requestTimeout
	client.pollInterval = pollInterval
	client.pollTimeout = pollTimeout
	client.httpClient = &http.Client{Timeout: requestTimeout}
	client.streamHTTPClient = newStreamHTTPClient(requestTimeout)
	return client
}

func testIngestRequest(conversationID string) api.IngestRequest {
	return api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: conversationID, ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:" + conversationID + ":1",
		Timestamp:      "2026-03-07T00:00:00Z",
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}
}
