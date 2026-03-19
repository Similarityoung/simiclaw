package runtime

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	"github.com/similarityyoung/simiclaw/internal/runtime/lanes"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

type EventLoop struct {
	facts     kernel.Facts
	processor *kernel.Service
	queue     chan runtimemodel.WorkItem
	scheduler *lanes.Scheduler
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
		facts:     facts,
		queue:     make(chan runtimemodel.WorkItem, queueCap),
		scheduler: lanes.NewScheduler(),
		ctx:       ctx,
		cancel:    cancel,
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
	return l.tryEnqueueWork(newEventWorkItem(eventID))
}

func (l *EventLoop) tryEnqueueWork(work runtimemodel.WorkItem) bool {
	select {
	case l.queue <- normalizeWorkItem(work):
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
			for _, work := range items {
				if !l.tryEnqueueWork(work) {
					break
				}
			}
		}
	}
}

func (l *EventLoop) processWork(work runtimemodel.WorkItem) {
	ctx, cancel := context.WithTimeout(l.ctx, 2*time.Minute)
	defer cancel()
	work = l.prepareWorkForScheduling(ctx, normalizeWorkItem(work))

	if l.scheduler != nil {
		lease, err := l.scheduler.Acquire(ctx, work)
		if err != nil {
			return
		}
		defer lease.Release()
	}

	_ = l.processor.Process(ctx, work)
}

func normalizeWorkItem(work runtimemodel.WorkItem) runtimemodel.WorkItem {
	if work.Kind == "" {
		work.Kind = runtimemodel.WorkKindEvent
	}
	if work.Identity == "" {
		switch work.Kind {
		case runtimemodel.WorkKindEvent, runtimemodel.WorkKindRecovery:
			work.Identity = work.EventID
		case runtimemodel.WorkKindOutbox:
			work.Identity = work.OutboxID
		case runtimemodel.WorkKindScheduledJob:
			work.Identity = work.JobID
		}
	}
	return work
}

func (l *EventLoop) prepareWorkForScheduling(ctx context.Context, work runtimemodel.WorkItem) runtimemodel.WorkItem {
	if work.SessionKey == "" && l.facts != nil {
		switch work.Kind {
		case runtimemodel.WorkKindEvent, runtimemodel.WorkKindRecovery:
			eventID := work.EventID
			if eventID == "" {
				eventID = work.Identity
			}
			if rec, ok, err := l.facts.GetEventRecord(ctx, eventID); err == nil && ok {
				work.SessionKey = rec.SessionKey
			}
		}
	}
	if work.LaneKey == "" {
		work.LaneKey = string(lanes.Resolve(work))
	}
	return work
}
