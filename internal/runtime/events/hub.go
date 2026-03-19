package events

import (
	"context"
	"sync"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

const (
	defaultSubscriptionBuffer = 64
	defaultReplayBuffer       = 2048
	defaultTerminalRetention  = 10 * time.Minute
)

type Subscription struct {
	id             uint64
	idempotencyKey string

	mu        sync.Mutex
	pending   []queuedEvent
	notify    chan struct{}
	closed    bool
	eventID   string
	maxQueued int
}

type queuedEvent struct {
	event     runtimemodel.RuntimeEvent
	droppable bool
}

type eventState struct {
	nextSequence int64
	subscribers  map[*Subscription]struct{}
	history      []queuedEvent
	terminal     *runtimemodel.RuntimeEvent
	updatedAt    time.Time
}

type Hub struct {
	mu                sync.Mutex
	events            map[string]*eventState
	reservations      map[*Subscription]struct{}
	nextSubscription  uint64
	terminalRetention time.Duration
	maxQueued         int
	maxReplay         int
}

func NewHub() *Hub {
	return &Hub{
		events:            make(map[string]*eventState),
		reservations:      make(map[*Subscription]struct{}),
		terminalRetention: defaultTerminalRetention,
		maxQueued:         defaultSubscriptionBuffer,
		maxReplay:         defaultReplayBuffer,
	}
}

func (h *Hub) Reserve(idempotencyKey string) *Subscription {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pruneLocked(time.Now().UTC())
	h.nextSubscription++
	sub := &Subscription{
		id:             h.nextSubscription,
		idempotencyKey: idempotencyKey,
		notify:         make(chan struct{}, 1),
		maxQueued:      h.maxQueued,
	}
	h.reservations[sub] = struct{}{}
	return sub
}

func (h *Hub) Release(sub *Subscription) {
	if sub == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.reservations, sub)
	if sub.eventID != "" {
		if state, ok := h.events[sub.eventID]; ok {
			delete(state.subscribers, sub)
			state.updatedAt = time.Now().UTC()
			if len(state.subscribers) == 0 && state.terminal != nil {
				h.pruneLocked(time.Now().UTC())
			}
		}
	}
	sub.close()
}

func (h *Hub) Attach(sub *Subscription, eventID string) []runtimemodel.RuntimeEvent {
	if sub == nil {
		return nil
	}
	now := time.Now().UTC()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pruneLocked(now)
	delete(h.reservations, sub)
	state := h.ensureEventLocked(eventID, now)
	sub.eventID = eventID
	replay := state.replay()
	if state.terminal == nil {
		state.subscribers[sub] = struct{}{}
	}
	state.updatedAt = now
	return replay
}

func (h *Hub) Publish(_ context.Context, event runtimemodel.RuntimeEvent) error {
	if event.EventID == "" {
		return nil
	}
	h.publish(event, event.IsTerminal())
	return nil
}

func (h *Hub) PublishTerminal(event runtimemodel.RuntimeEvent) runtimemodel.RuntimeEvent {
	return h.publish(event, true)
}

func (h *Hub) publish(event runtimemodel.RuntimeEvent, terminal bool) runtimemodel.RuntimeEvent {
	now := time.Now().UTC()
	eventID := event.EventID
	if eventID == "" {
		return event
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pruneLocked(now)
	state := h.ensureEventLocked(eventID, now)
	if terminal && state.terminal != nil {
		cached := *state.terminal
		return cached
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = now
	}
	if event.Sequence == 0 {
		event.Sequence = state.nextSequence
		state.nextSequence++
	}
	state.updatedAt = now
	queueDroppable := event.Kind == runtimemodel.RuntimeEventClaimed ||
		event.Kind == runtimemodel.RuntimeEventExecuting ||
		event.Kind == runtimemodel.RuntimeEventFinalizeStarted ||
		event.Kind == runtimemodel.RuntimeEventReasoningDelta ||
		event.Kind == runtimemodel.RuntimeEventTextDelta
	replayDroppable := event.Kind == runtimemodel.RuntimeEventReasoningDelta ||
		event.Kind == runtimemodel.RuntimeEventTextDelta
	if !terminal {
		state.appendHistory(queuedEvent{event: event, droppable: replayDroppable}, h.maxReplay)
	}
	for sub := range state.subscribers {
		if terminal {
			sub.enqueue(event, false)
			sub.close()
			continue
		}
		sub.enqueue(event, queueDroppable)
	}
	if terminal {
		cached := event
		state.terminal = &cached
		for sub := range state.subscribers {
			delete(state.subscribers, sub)
		}
	}
	return event
}

func (h *Hub) ensureEventLocked(eventID string, now time.Time) *eventState {
	state, ok := h.events[eventID]
	if !ok {
		state = &eventState{
			nextSequence: 2,
			subscribers:  make(map[*Subscription]struct{}),
			updatedAt:    now,
		}
		h.events[eventID] = state
	}
	return state
}

func (s *eventState) appendHistory(item queuedEvent, maxQueued int) {
	s.history = append(s.history, item)
	if maxQueued <= 0 || len(s.history) <= maxQueued {
		return
	}
	filtered := s.history[:0]
	for _, existing := range s.history {
		if len(filtered) < maxQueued || !existing.droppable {
			filtered = append(filtered, existing)
		}
	}
	s.history = filtered
	if len(s.history) <= maxQueued {
		return
	}
	s.history = append([]queuedEvent(nil), s.history[len(s.history)-maxQueued:]...)
}

func (s *eventState) replay() []runtimemodel.RuntimeEvent {
	if len(s.history) == 0 && s.terminal == nil {
		return nil
	}
	events := make([]runtimemodel.RuntimeEvent, 0, len(s.history)+1)
	for _, item := range s.history {
		events = append(events, item.event)
	}
	if s.terminal != nil {
		events = append(events, *s.terminal)
	}
	return events
}

func (h *Hub) pruneLocked(now time.Time) {
	for eventID, state := range h.events {
		if state.terminal == nil || len(state.subscribers) > 0 {
			continue
		}
		if now.Sub(state.updatedAt) > h.terminalRetention {
			delete(h.events, eventID)
		}
	}
}

func (s *Subscription) Next(ctx context.Context) (runtimemodel.RuntimeEvent, bool) {
	for {
		s.mu.Lock()
		if len(s.pending) > 0 {
			item := s.pending[0]
			copy(s.pending, s.pending[1:])
			s.pending = s.pending[:len(s.pending)-1]
			s.mu.Unlock()
			return item.event, true
		}
		if s.closed {
			s.mu.Unlock()
			return runtimemodel.RuntimeEvent{}, false
		}
		notify := s.notify
		s.mu.Unlock()

		select {
		case <-ctx.Done():
			return runtimemodel.RuntimeEvent{}, false
		case <-notify:
		}
	}
}

func (s *Subscription) enqueue(event runtimemodel.RuntimeEvent, droppable bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	if len(s.pending) >= s.maxQueued {
		if droppable {
			return false
		}
		filtered := s.pending[:0]
		for _, item := range s.pending {
			if item.droppable {
				continue
			}
			filtered = append(filtered, item)
		}
		s.pending = filtered
	}
	s.pending = append(s.pending, queuedEvent{event: event, droppable: droppable})
	select {
	case s.notify <- struct{}{}:
	default:
	}
	return true
}

func (s *Subscription) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	select {
	case s.notify <- struct{}{}:
	default:
	}
}
