//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestRuntimeKernelLifecycleStreamAndPersistence(t *testing.T) {
	app := newTestApp(t)
	req := api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "integration-kernel", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:integration-kernel:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello kernel"},
	}

	resp := ingest(t, app, req, 202)

	event := pollEvent(t, app, resp.EventID)
	if event.Status != model.EventStatusProcessed {
		t.Fatalf("expected persisted event to be processed, got %+v", event)
	}
	if event.OutboxStatus != model.OutboxStatusSent {
		t.Fatalf("expected durable outbox send after finalize, got %+v", event)
	}

	sub := app.StreamHub.Reserve(req.IdempotencyKey + ":terminal-replay")
	defer app.StreamHub.Release(sub)
	terminal := app.StreamHub.Attach(sub, resp.EventID)
	if terminal == nil {
		t.Fatalf("expected terminal replay after event finished, event=%+v", event)
	}
	if terminal.Type != api.ChatStreamEventDone {
		t.Fatalf("expected done terminal replay, got %+v", terminal)
	}
	if terminal.EventRecord == nil {
		t.Fatalf("expected terminal replay to carry event record, got %+v", terminal)
	}
	if terminal.EventRecord.EventID != event.EventID || terminal.EventRecord.RunID != event.RunID {
		t.Fatalf("expected terminal replay to align with persisted event, terminal=%+v event=%+v", terminal.EventRecord, event)
	}
	if terminal.EventRecord.Status != model.EventStatusProcessed {
		t.Fatalf("expected terminal replay status to be processed, got %+v", terminal.EventRecord)
	}

	trace := getRunTrace(t, app, event.RunID)
	if trace.RunID != event.RunID || trace.EventID != event.EventID {
		t.Fatalf("expected run trace to align with persisted event, got trace=%+v event=%+v", trace, event)
	}
	if trace.Status != model.RunStatusCompleted || trace.RunMode != model.RunModeNormal {
		t.Fatalf("expected completed normal run trace, got %+v", trace)
	}
}
