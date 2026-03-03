package bus

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

var ErrBusClosed = errors.New("message bus closed")

type MessageBus struct {
	inbound chan model.InternalEvent
	done    chan struct{}
	closed  atomic.Bool
}

// NewMessageBus creates a new MessageBus with the specified capacity for inbound events.
func NewMessageBus(inboundCapacity int) *MessageBus {
	if inboundCapacity <= 0 {
		inboundCapacity = 1024
	}
	return &MessageBus{
		inbound: make(chan model.InternalEvent, inboundCapacity),
		done:    make(chan struct{}),
	}
}

func (b *MessageBus) PublishInbound(ctx context.Context, event model.InternalEvent) error {
	if b.closed.Load() {
		return ErrBusClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case b.inbound <- event:
		return nil
	case <-b.done:
		return ErrBusClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *MessageBus) TryPublishInbound(event model.InternalEvent) bool {
	if b.closed.Load() {
		return false
	}
	select {
	case b.inbound <- event:
		return true
	default:
		return false
	}
}

func (b *MessageBus) ConsumeInbound(ctx context.Context) (model.InternalEvent, bool) {
	// Drain any buffered event first; this allows graceful shutdown to process
	// already-accepted events before honoring the close signal.
	select {
	case evt, ok := <-b.inbound:
		if !ok {
			return model.InternalEvent{}, false
		}
		return evt, true
	default:
	}

	select {
	case evt, ok := <-b.inbound:
		if !ok {
			return model.InternalEvent{}, false
		}
		return evt, true
	case <-b.done:
		return model.InternalEvent{}, false
	case <-ctx.Done():
		return model.InternalEvent{}, false
	}
}

func (b *MessageBus) InboundDepth() int {
	return len(b.inbound)
}

func (b *MessageBus) InboundCapacity() int {
	return cap(b.inbound)
}

func (b *MessageBus) Close() {
	if b.closed.CompareAndSwap(false, true) {
		close(b.done)
	}
}
