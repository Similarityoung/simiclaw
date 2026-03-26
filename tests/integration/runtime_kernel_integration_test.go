//go:build integration

package integration

import (
	"testing"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
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

	sub := app.StreamHub.Reserve()
	defer app.StreamHub.Release(sub)
	replay := app.StreamHub.Attach(sub, resp.EventID)
	if len(replay) == 0 {
		t.Fatalf("expected terminal replay after event finished, event=%+v", event)
	}
	assertRuntimeReplayPath(t, replay, []string{"claimed", "running", "finalizing"})
	terminal := replay[len(replay)-1]
	if terminal.Kind != runtimemodel.RuntimeEventCompleted {
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

func assertRuntimeReplayPath(t *testing.T, replay []runtimemodel.RuntimeEvent, want []string) {
	t.Helper()
	idx := 0
	for _, event := range replay {
		if idx < len(want) && event.Message == want[idx] {
			idx++
		}
	}
	if idx != len(want) {
		t.Fatalf("expected replay path %v, got %+v", want, replay)
	}
}
