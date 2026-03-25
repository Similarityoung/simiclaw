package events

import (
	"context"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

// TerminalReplaySource provides terminal retention fallback for observers that
// attach after runtime execution has already finished or after in-memory replay
// has been lost.
type TerminalReplaySource interface {
	GetTerminalEvent(ctx context.Context, eventID string) (runtimemodel.RuntimeEvent, bool, error)
}

// StreamObserver is the runtime-owned observe seam consumed by surface stream
// adapters.
type StreamObserver interface {
	Open(idempotencyKey string) StreamSubscription
}

// StreamSubscription models one observer attached to a single event stream.
type StreamSubscription interface {
	Attach(ctx context.Context, eventID string) []runtimemodel.RuntimeEvent
	Next(ctx context.Context) (runtimemodel.RuntimeEvent, bool)
	Close()
}

type Observer struct {
	hub      *Hub
	terminal TerminalReplaySource
}

func NewObserver(hub *Hub, terminal TerminalReplaySource) *Observer {
	if hub == nil {
		hub = NewHub()
	}
	return &Observer{hub: hub, terminal: terminal}
}

func (o *Observer) Open(idempotencyKey string) StreamSubscription {
	return &observerSubscription{
		observer: o,
		sub:      o.hub.Reserve(idempotencyKey),
	}
}

type observerSubscription struct {
	observer *Observer
	sub      *Subscription
}

func (s *observerSubscription) Attach(ctx context.Context, eventID string) []runtimemodel.RuntimeEvent {
	replay := s.observer.hub.Attach(s.sub, eventID)
	if replayContainsTerminal(replay) || s.observer.terminal == nil {
		return replay
	}
	terminal, ok, err := s.observer.terminal.GetTerminalEvent(ctx, eventID)
	if err != nil || !ok {
		return replay
	}
	return append(replay, s.observer.hub.PublishTerminal(terminal))
}

func (s *observerSubscription) Next(ctx context.Context) (runtimemodel.RuntimeEvent, bool) {
	return s.sub.Next(ctx)
}

func (s *observerSubscription) Close() {
	s.observer.hub.Release(s.sub)
}

func replayContainsTerminal(replay []runtimemodel.RuntimeEvent) bool {
	for _, event := range replay {
		if event.IsTerminal() {
			return true
		}
	}
	return false
}
