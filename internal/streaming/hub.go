package streaming

import (
	"context"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"
)

const (
	defaultSubscriptionBuffer = 64
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
	event     api.ChatStreamEvent
	droppable bool
}

type eventState struct {
	nextSequence int64
	subscribers  map[*Subscription]struct{}
	terminal     *api.ChatStreamEvent
	updatedAt    time.Time
}

type Hub struct {
	mu                sync.Mutex
	events            map[string]*eventState
	reservations      map[*Subscription]struct{}
	nextSubscription  uint64
	terminalRetention time.Duration
	maxQueued         int
}

func NewHub() *Hub {
	return &Hub{
		events:            make(map[string]*eventState),
		reservations:      make(map[*Subscription]struct{}),
		terminalRetention: defaultTerminalRetention,
		maxQueued:         defaultSubscriptionBuffer,
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

func (h *Hub) Attach(sub *Subscription, eventID string) *api.ChatStreamEvent {
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
	if state.terminal != nil {
		terminal := *state.terminal
		return &terminal
	}
	state.subscribers[sub] = struct{}{}
	state.updatedAt = now
	return nil
}

func (h *Hub) Publish(eventID string, event api.ChatStreamEvent) api.ChatStreamEvent {
	return h.publish(eventID, event, false)
}

func (h *Hub) PublishTerminal(eventID string, event api.ChatStreamEvent) api.ChatStreamEvent {
	return h.publish(eventID, event, true)
}

func (h *Hub) publish(eventID string, event api.ChatStreamEvent, terminal bool) api.ChatStreamEvent {
	now := time.Now().UTC()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pruneLocked(now)
	state := h.ensureEventLocked(eventID, now)
	if terminal && state.terminal != nil {
		cached := *state.terminal
		return cached
	}
	event.EventID = eventID
	if event.At.IsZero() {
		event.At = now
	}
	if event.Sequence == 0 {
		event.Sequence = state.nextSequence
		state.nextSequence++
	}
	state.updatedAt = now
	droppable := event.Type == api.ChatStreamEventStatus ||
		event.Type == api.ChatStreamEventReasoningDelta ||
		event.Type == api.ChatStreamEventTextDelta
	for sub := range state.subscribers {
		if terminal {
			sub.enqueue(event, false)
			sub.close()
			continue
		}
		sub.enqueue(event, droppable)
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

func (s *Subscription) Next(ctx context.Context) (api.ChatStreamEvent, bool) {
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
			return api.ChatStreamEvent{}, false
		}
		notify := s.notify
		s.mu.Unlock()

		select {
		case <-ctx.Done():
			return api.ChatStreamEvent{}, false
		case <-notify:
		}
	}
}

func (s *Subscription) enqueue(event api.ChatStreamEvent, droppable bool) bool {
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
