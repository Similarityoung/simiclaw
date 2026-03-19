//go:build integration

package integration

import (
	"testing"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestRuntimeLaneHooksPreserveLifecycleAndExposeSessionLane(t *testing.T) {
	app := newTestApp(t)
	req := api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "runtime-lanes", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:runtime-lanes:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "lane ready"},
	}

	resp := ingest(t, app, req, 202)
	event := pollEvent(t, app, resp.EventID)
	if event.Status != model.EventStatusProcessed {
		t.Fatalf("expected processed event, got %+v", event)
	}
	if event.OutboxStatus != model.OutboxStatusSent {
		t.Fatalf("expected durable delivery after finalize, got %+v", event)
	}

	sub := app.StreamHub.Reserve(req.IdempotencyKey + ":lane-replay")
	defer app.StreamHub.Release(sub)
	replay := app.StreamHub.Attach(sub, resp.EventID)
	if len(replay) == 0 {
		t.Fatalf("expected runtime replay for lane verification")
	}

	expectedLane := "session:" + event.SessionKey
	assertRuntimeReplayPath(t, replay, []string{"claimed", "running", "finalizing"})
	for _, runtimeEvent := range replay {
		switch runtimeEvent.Kind {
		case runtimemodel.RuntimeEventClaimed,
			runtimemodel.RuntimeEventExecuting,
			runtimemodel.RuntimeEventFinalizeStarted,
			runtimemodel.RuntimeEventCompleted,
			runtimemodel.RuntimeEventFailed:
			if runtimeEvent.Work.LaneKey != expectedLane {
				t.Fatalf("expected runtime event %s to use lane %q, got %+v", runtimeEvent.Kind, expectedLane, runtimeEvent.Work)
			}
		}
	}

	terminal := replay[len(replay)-1]
	if terminal.Kind != runtimemodel.RuntimeEventCompleted {
		t.Fatalf("expected completed terminal replay, got %+v", terminal)
	}
	if terminal.EventRecord == nil || terminal.EventRecord.Status != model.EventStatusProcessed {
		t.Fatalf("expected processed terminal record, got %+v", terminal.EventRecord)
	}
}
