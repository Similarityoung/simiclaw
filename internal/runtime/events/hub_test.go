package events

import (
	"context"
	"testing"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

func TestHubPublishesSharedSequenceToSubscribers(t *testing.T) {
	hub := NewHub()
	sub1 := hub.Reserve()
	defer hub.Release(sub1)
	sub2 := hub.Reserve()
	defer hub.Release(sub2)

	if replay := hub.Attach(sub1, "evt_1"); len(replay) > 0 {
		t.Fatalf("unexpected replay for sub1: %+v", replay)
	}
	if replay := hub.Attach(sub2, "evt_1"); len(replay) > 0 {
		t.Fatalf("unexpected replay for sub2: %+v", replay)
	}

	if err := hub.Publish(context.Background(), runtimemodel.RuntimeEvent{
		Kind:       runtimemodel.RuntimeEventClaimed,
		EventID:    "evt_1",
		Status:     "processing",
		Message:    "claimed",
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Publish: %v", err)
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
	if ev1.Sequence != 2 || ev2.Sequence != 2 {
		t.Fatalf("expected first live sequence 2, got ev1=%d ev2=%d", ev1.Sequence, ev2.Sequence)
	}
}

func TestHubReplaysTerminalToLateSubscriber(t *testing.T) {
	hub := NewHub()
	terminal := hub.PublishTerminal(runtimemodel.RuntimeEvent{
		Kind:       runtimemodel.RuntimeEventCompleted,
		EventID:    "evt_2",
		OccurredAt: time.Now().UTC(),
	})
	if terminal.Sequence != 2 {
		t.Fatalf("expected terminal sequence 2, got %d", terminal.Sequence)
	}

	sub := hub.Reserve()
	defer hub.Release(sub)
	replayed := hub.Attach(sub, "evt_2")
	if len(replayed) != 1 {
		t.Fatalf("expected replayed terminal")
	}
	if replayed[0].Kind != runtimemodel.RuntimeEventCompleted {
		t.Fatalf("expected done terminal, got %+v", replayed)
	}
	if replayed[0].Sequence != terminal.Sequence {
		t.Fatalf("expected same sequence, got %d want %d", replayed[0].Sequence, terminal.Sequence)
	}
}

func TestHubReplaysPreAttachHistoryInOrder(t *testing.T) {
	hub := NewHub()
	if err := hub.Publish(context.Background(), runtimemodel.RuntimeEvent{
		Kind:       runtimemodel.RuntimeEventClaimed,
		EventID:    "evt_3",
		Message:    "claimed",
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Publish claimed: %v", err)
	}
	terminal := hub.PublishTerminal(runtimemodel.RuntimeEvent{
		Kind:       runtimemodel.RuntimeEventCompleted,
		EventID:    "evt_3",
		OccurredAt: time.Now().UTC(),
	})

	sub := hub.Reserve()
	defer hub.Release(sub)
	replay := hub.Attach(sub, "evt_3")
	if len(replay) != 2 {
		t.Fatalf("expected claimed + terminal replay, got %+v", replay)
	}
	if replay[0].Kind != runtimemodel.RuntimeEventClaimed {
		t.Fatalf("expected first replayed event to be claimed, got %+v", replay)
	}
	if replay[1].Kind != runtimemodel.RuntimeEventCompleted {
		t.Fatalf("expected terminal replay, got %+v", replay)
	}
	if replay[1].Sequence != terminal.Sequence {
		t.Fatalf("expected terminal sequence %d, got %d", terminal.Sequence, replay[1].Sequence)
	}
}

func TestHubDropsQueueDroppableWhenSubscriberBufferIsFull(t *testing.T) {
	hub := NewHub()
	hub.maxQueued = 1

	sub := hub.Reserve()
	defer hub.Release(sub)
	_ = hub.Attach(sub, "evt_qdrop")

	if err := hub.Publish(context.Background(), runtimemodel.RuntimeEvent{
		Kind:    runtimemodel.RuntimeEventTextDelta,
		EventID: "evt_qdrop",
		Delta:   "first",
	}); err != nil {
		t.Fatalf("Publish first delta: %v", err)
	}
	if err := hub.Publish(context.Background(), runtimemodel.RuntimeEvent{
		Kind:    runtimemodel.RuntimeEventTextDelta,
		EventID: "evt_qdrop",
		Delta:   "second",
	}); err != nil {
		t.Fatalf("Publish second delta: %v", err)
	}
	terminal := hub.PublishTerminal(runtimemodel.RuntimeEvent{
		Kind:    runtimemodel.RuntimeEventCompleted,
		EventID: "evt_qdrop",
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	first, ok := sub.Next(ctx)
	if !ok {
		t.Fatalf("expected first event from subscription")
	}
	if first.Kind != runtimemodel.RuntimeEventCompleted {
		t.Fatalf("expected terminal event, got %+v", first)
	}
	if first.Sequence != terminal.Sequence {
		t.Fatalf("expected terminal sequence %d, got %d", terminal.Sequence, first.Sequence)
	}
	if _, ok := sub.Next(ctx); ok {
		t.Fatalf("expected subscription to close after terminal")
	}
}
