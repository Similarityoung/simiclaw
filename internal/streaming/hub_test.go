package streaming

import (
	"context"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestHubPublishesSharedSequenceToSubscribers(t *testing.T) {
	hub := NewHub()
	sub1 := hub.Reserve("k1")
	defer hub.Release(sub1)
	sub2 := hub.Reserve("k2")
	defer hub.Release(sub2)

	if terminal := hub.Attach(sub1, "evt_1"); terminal != nil {
		t.Fatalf("unexpected terminal for sub1: %+v", terminal)
	}
	if terminal := hub.Attach(sub2, "evt_1"); terminal != nil {
		t.Fatalf("unexpected terminal for sub2: %+v", terminal)
	}

	published := hub.Publish("evt_1", model.ChatStreamEvent{
		Type:    model.ChatStreamEventStatus,
		Status:  "processing",
		Message: "claimed",
	})
	if published.Sequence != 2 {
		t.Fatalf("expected first live sequence 2, got %d", published.Sequence)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ev1, ok := sub1.Next(ctx)
	if !ok {
		t.Fatalf("sub1 did not receive event")
	}
	ev2, ok := sub2.Next(ctx)
	if !ok {
		t.Fatalf("sub2 did not receive event")
	}
	if ev1.Sequence != published.Sequence || ev2.Sequence != published.Sequence {
		t.Fatalf("sequence mismatch: published=%d ev1=%d ev2=%d", published.Sequence, ev1.Sequence, ev2.Sequence)
	}
}

func TestHubReplaysTerminalToLateSubscriber(t *testing.T) {
	hub := NewHub()
	terminal := hub.PublishTerminal("evt_2", model.ChatStreamEvent{
		Type: model.ChatStreamEventDone,
		EventRecord: &model.EventRecord{
			EventID: "evt_2",
			Status:  model.EventStatusProcessed,
		},
	})
	if terminal.Sequence != 2 {
		t.Fatalf("expected terminal sequence 2, got %d", terminal.Sequence)
	}

	sub := hub.Reserve("k3")
	defer hub.Release(sub)
	replayed := hub.Attach(sub, "evt_2")
	if replayed == nil {
		t.Fatalf("expected replayed terminal")
	}
	if replayed.Type != model.ChatStreamEventDone {
		t.Fatalf("expected done terminal, got %+v", replayed)
	}
	if replayed.Sequence != terminal.Sequence {
		t.Fatalf("expected same sequence, got %d want %d", replayed.Sequence, terminal.Sequence)
	}
}
