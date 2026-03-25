package runtime

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	"github.com/similarityyoung/simiclaw/internal/runtime/lanes"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/logging"
)

type EventLoop struct {
	facts     kernel.Facts
	processor kernel.Processor
	queue     chan runtimemodel.WorkItem
	scheduler *lanes.Scheduler
	logger    *logging.Logger
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
	loop := &EventLoop{
		facts:     facts,
		queue:     make(chan runtimemodel.WorkItem, queueCap),
		scheduler: lanes.NewScheduler(),
		logger:    logging.L("runtime.eventloop"),
	}
	processor := kernel.NewService(facts, executor, events)
	processor.SetClock(func() time.Time { return time.Now().UTC() })
	processor.SetIDGenerator(func() uint64 { return loop.enqueueID.Add(1) })
	loop.processor = processor
	return loop
}

func (l *EventLoop) Start(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("runtime event loop requires a non-nil context")
	}
	l.ctx, l.cancel = context.WithCancel(ctx)
	l.alive.Store(true)
	l.logger.Info("event loop started", logging.Int("queue_capacity", cap(l.queue)))
	l.wg.Add(2)
	go l.consumeLoop()
	go l.repumpLoop()
	return nil
}

func (l *EventLoop) Stop() {
	if l.cancel != nil {
		l.cancel()
	}
	l.wg.Wait()
	l.alive.Store(false)
	l.logger.Info("event loop stopped")
}

func (l *EventLoop) IsAlive() bool {
	return l.alive.Load()
}

func (l *EventLoop) TryEnqueue(eventID string) bool {
	return l.tryEnqueueWork(runtimemodel.WorkItem{EventID: eventID})
}

func (l *EventLoop) tryEnqueueWork(work runtimemodel.WorkItem) bool {
	logger := l.logger.With(workLogFields(work)...)
	select {
	case l.queue <- work:
		logger.Info("work enqueued", logging.Int("queue_depth", len(l.queue)))
		return true
	default:
		logger.Warn("work deferred", logging.Int("queue_depth", len(l.queue)))
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
		case work := <-l.queue:
			l.processWork(work)
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
			if len(items) == 0 {
				continue
			}
			enqueued := 0
			for _, work := range items {
				if !l.tryEnqueueWork(work) {
					break
				}
				enqueued++
			}
			l.logger.Info("repump enqueued",
				logging.Int("count", len(items)),
				logging.Int("enqueued", enqueued),
				logging.Int("deferred", len(items)-enqueued),
			)
		}
	}
}

func (l *EventLoop) processWork(work runtimemodel.WorkItem) {
	ctx, cancel := context.WithTimeout(l.ctx, 2*time.Minute)
	defer cancel()
	work = l.prepareWorkForScheduling(ctx, work)

	if l.scheduler != nil {
		lease, err := l.scheduler.Acquire(ctx, work)
		if err != nil {
			return
		}
		defer lease.Release()
	}

	_ = l.processor.Process(ctx, work)
}

func workLogFields(work runtimemodel.WorkItem) []logging.Field {
	fields := make([]logging.Field, 0, 3)
	if work.EventID != "" {
		fields = append(fields, logging.String("event_id", work.EventID))
	}
	if work.SessionKey != "" {
		fields = append(fields, logging.String("session_key", work.SessionKey))
	}
	if work.LaneKey != "" {
		fields = append(fields, logging.String("lane_key", work.LaneKey))
	}
	return fields
}

func (l *EventLoop) prepareWorkForScheduling(ctx context.Context, work runtimemodel.WorkItem) runtimemodel.WorkItem {
	if work.SessionKey == "" && l.facts != nil && work.EventID != "" {
		if rec, ok, err := l.facts.GetEventRecord(ctx, work.EventID); err == nil && ok {
			work.SessionKey = rec.SessionKey
		}
	}
	if work.LaneKey == "" {
		work.LaneKey = string(lanes.Resolve(work))
	}
	return work
}
