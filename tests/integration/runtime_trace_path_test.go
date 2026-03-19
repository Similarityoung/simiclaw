//go:build integration

package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestRuntimeTracePathExposesClaimExecuteFinalizeAndDelivery(t *testing.T) {
	app := newTestApp(t)
	server := httptest.NewServer(app.Handler)
	defer server.Close()

	req := api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "trace-path", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:trace-path:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "trace this path"},
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := http.Post(server.URL+"/v1/chat:stream", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("chat stream request: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", got)
	}

	reader := bufio.NewReader(resp.Body)
	accepted := readStreamEvent(t, reader)
	if accepted.Type != api.ChatStreamEventAccepted {
		t.Fatalf("expected accepted event, got %+v", accepted)
	}

	var (
		statuses []string
		sawText  bool
		done     api.ChatStreamEvent
	)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		event := readStreamEvent(t, reader)
		switch event.Type {
		case api.ChatStreamEventStatus:
			statuses = append(statuses, event.Message)
		case api.ChatStreamEventTextDelta:
			sawText = true
		case api.ChatStreamEventDone:
			done = event
			goto complete
		case api.ChatStreamEventError:
			t.Fatalf("unexpected terminal error: %+v", event)
		}
	}
	t.Fatalf("timeout waiting for done event")

complete:
	if !sawText {
		t.Fatalf("expected at least one text_delta event")
	}
	assertStatusPath(t, statuses, []string{"claimed", "running", "finalizing"})
	if done.EventRecord == nil {
		t.Fatalf("done missing event record: %+v", done)
	}
	if done.EventRecord.EventID != accepted.EventID {
		t.Fatalf("event id mismatch: accepted=%s done=%+v", accepted.EventID, done.EventRecord)
	}
	if done.EventRecord.Status != model.EventStatusProcessed {
		t.Fatalf("expected processed terminal event record, got %+v", done.EventRecord)
	}
	if done.EventRecord.OutboxStatus != "" &&
		done.EventRecord.OutboxStatus != model.OutboxStatusPending &&
		done.EventRecord.OutboxStatus != model.OutboxStatusSent {
		t.Fatalf("unexpected terminal outbox status: %+v", done.EventRecord)
	}

	final := pollEvent(t, app, accepted.EventID)
	if final.Status != model.EventStatusProcessed {
		t.Fatalf("expected processed final event, got %+v", final)
	}
	if final.OutboxStatus != model.OutboxStatusSent {
		t.Fatalf("expected delivery to complete after finalize, got %+v", final)
	}
}

func assertStatusPath(t *testing.T, got []string, want []string) {
	t.Helper()
	idx := 0
	for _, item := range got {
		if idx < len(want) && item == want[idx] {
			idx++
		}
	}
	if idx != len(want) {
		t.Fatalf("expected status path %v, got %v", want, got)
	}
}
