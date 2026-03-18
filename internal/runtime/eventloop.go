package runtime

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
)

type EventLoop struct {
	facts     kernel.Facts
	processor *kernel.Service
	queue     chan string
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	alive     atomic.Bool
	enqueueID atomic.Uint64
}

func NewEventLoop(facts kernel.Facts, executor kernel.Executor, events kernel.EventSink, queueCap int) *EventLoop {
	if queueCap <= 0 {
		queueCap = 1024
	}
	ctx, cancel := context.WithCancel(context.Background())
	loop := &EventLoop{
		facts:  facts,
		queue:  make(chan string, queueCap),
		ctx:    ctx,
		cancel: cancel,
	}
	processor := kernel.NewService(facts, executor, events)
	processor.SetClock(func() time.Time { return time.Now().UTC() })
	processor.SetIDGenerator(func() uint64 { return loop.enqueueID.Add(1) })
	loop.processor = processor
	return loop
}

func (l *EventLoop) Start() {
	l.alive.Store(true)
	l.wg.Add(2)
	go l.consumeLoop()
	go l.repumpLoop()
}

func (l *EventLoop) Stop() {
	l.cancel()
	l.wg.Wait()
	l.alive.Store(false)
}

func (l *EventLoop) IsAlive() bool {
	return l.alive.Load()
}

func (l *EventLoop) TryEnqueue(eventID string) bool {
	select {
	case l.queue <- eventID:
		return true
	default:
		return false
	}
}

func (l *EventLoop) InboundDepth() int {
	return len(l.queue)
}

func (l *EventLoop) consumeLoop() {
	defer l.wg.Done()
	for {
		select {
		case <-l.ctx.Done():
			return
		case eventID := <-l.queue:
			l.processEvent(eventID)
		}
	}
}

func (l *EventLoop) repumpLoop() {
	defer l.wg.Done()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-l.ctx.Done():
			return
		case <-ticker.C:
			if l.facts == nil {
				continue
			}
			ctx, cancel := context.WithTimeout(l.ctx, time.Second)
			items, err := l.facts.ListRunnable(ctx, cap(l.queue))
			cancel()
			if err != nil {
				continue
			}
			for _, work := range items {
				eventID := work.EventID
				if eventID == "" {
					eventID = work.Identity
				}
				if eventID == "" {
					continue
				}
				select {
				case l.queue <- eventID:
				default:
					break
				}
			}
		}
	}
}

func (l *EventLoop) processEvent(eventID string) {
	ctx, cancel := context.WithTimeout(l.ctx, 2*time.Minute)
	defer cancel()
	_ = l.processor.Process(ctx, newEventWorkItem(eventID))
}
