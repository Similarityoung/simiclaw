package runtime

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/similarityyoung/simiclaw/internal/runner"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/internal/streaming"
)

type EventLoop struct {
	repo      EventLoopRepository
	processor *kernel.Service
	maxRounds int
	queue     chan string
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	alive     atomic.Bool
	enqueueID atomic.Uint64
}

type EventLoopRepository interface {
	ListRunnableEventIDs(ctx context.Context, limit int) ([]string, error)
	ClaimLoopEvent(ctx context.Context, eventID, runID string, now time.Time) (runtimemodel.ClaimedEvent, bool, error)
	FinalizeLoopRun(ctx context.Context, finalize runtimemodel.RunFinalize) error
	GetLoopEventRecord(ctx context.Context, eventID string) (runtimemodel.EventRecord, bool, error)
}

func NewEventLoop(repo EventLoopRepository, run runner.Runner, streamHub *streaming.Hub, queueCap, maxRounds int) *EventLoop {
	if queueCap <= 0 {
		queueCap = 1024
	}
	if maxRounds <= 0 {
		maxRounds = 4
	}
	ctx, cancel := context.WithCancel(context.Background())
	loop := &EventLoop{
		repo:      repo,
		maxRounds: maxRounds,
		queue:     make(chan string, queueCap),
		ctx:       ctx,
		cancel:    cancel,
	}
	facts := eventLoopFactsAdapter{repo: repo}
	processor := kernel.NewService(facts, runnerExecutor{
		runner:    run,
		maxRounds: maxRounds,
		streamHub: streamHub,
	}, newHubRuntimeEventSink(streamHub, facts))
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
			ctx, cancel := context.WithTimeout(l.ctx, time.Second)
			ids, err := l.repo.ListRunnableEventIDs(ctx, cap(l.queue))
			cancel()
			if err != nil {
				continue
			}
			for _, id := range ids {
				select {
				case l.queue <- id:
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
