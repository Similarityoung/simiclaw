package events

import (
	"context"
	"testing"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type fakeTerminalReplaySource struct {
	event runtimemodel.RuntimeEvent
	ok    bool
	err   error
}

func (f fakeTerminalReplaySource) GetTerminalEvent(context.Context, string) (runtimemodel.RuntimeEvent, bool, error) {
	return f.event, f.ok, f.err
}

func TestObserverAttachFallsBackToTerminalReplaySource(t *testing.T) {
	now := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)
	observer := NewObserver(NewHub(), fakeTerminalReplaySource{
		ok: true,
		event: runtimemodel.RuntimeEvent{
			Kind:       runtimemodel.RuntimeEventCompleted,
			EventID:    "evt_terminal",
			OccurredAt: now,
			EventRecord: &runtimemodel.EventRecord{
				EventID:        "evt_terminal",
				Status:         model.EventStatusProcessed,
				AssistantReply: "done",
				UpdatedAt:      now,
			},
		},
	})

	sub := observer.Open("idem-terminal")
	defer sub.Close()

	replay := sub.Attach(context.Background(), "evt_terminal")
	if len(replay) != 1 {
		t.Fatalf("expected one terminal replay event, got %+v", replay)
	}
	if !replay[0].IsTerminal() {
		t.Fatalf("expected terminal replay event, got %+v", replay[0])
	}
	if replay[0].EventRecord == nil || replay[0].EventRecord.AssistantReply != "done" {
		t.Fatalf("unexpected terminal replay payload: %+v", replay[0])
	}
}
