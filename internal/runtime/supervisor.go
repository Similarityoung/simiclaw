package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/ingest"
	"github.com/similarityyoung/simiclaw/internal/outbound"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	runtimeworkers "github.com/similarityyoung/simiclaw/internal/runtime/workers"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const heartbeatFreshness = 30 * time.Second

type Supervisor struct {
	cfg        config.Config
	workers    WorkerRepository
	readiness  ReadinessRepository
	ingest     EventIngestor
	loop       *EventLoop
	background []kernel.Worker
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

type EventIngestor interface {
	Ingest(ctx context.Context, cmd ingest.Command) (ingest.Result, *ingest.Error)
}

type WorkerRepository interface {
	UpsertCronJobs(ctx context.Context, tenantID string, jobs []config.CronJobConfig, now time.Time) error
	BeatHeartbeat(ctx context.Context, workerName string, now time.Time) error
	RecoverExpiredProcessing(ctx context.Context, cutoff, now time.Time) ([]string, error)
	RecoverExpiredSending(ctx context.Context, cutoff, now time.Time) error
	ClaimRuntimeOutbox(ctx context.Context, owner string, now time.Time) (runtimemodel.ClaimedOutbox, bool, error)
	FailOutboxSend(ctx context.Context, outboxID, eventID, message string, dead bool, nextAttemptAt, now time.Time) error
	CompleteOutboxSend(ctx context.Context, outboxID, eventID string, now time.Time) error
	ClaimRuntimeScheduledJob(ctx context.Context, kind model.ScheduledJobKind, owner string, now time.Time) (runtimemodel.ClaimedJob, bool, error)
	FailScheduledJob(ctx context.Context, jobID, message string, nextRunAt, now time.Time) error
	CompleteRuntimeScheduledJob(ctx context.Context, job runtimemodel.ClaimedJob, now time.Time) error
	MarkEventQueued(ctx context.Context, eventID string, now time.Time) error
}

type ReadinessRepository interface {
	CheckReadWrite(ctx context.Context) error
	HeartbeatAt(ctx context.Context, workerName string) (time.Time, bool, error)
}

func NewSupervisor(cfg config.Config, workers WorkerRepository, readiness ReadinessRepository, ingestor EventIngestor, loop *EventLoop, sender outbound.Sender) *Supervisor {
	ctx, cancel := context.WithCancel(context.Background())
	var enqueuer runtimeworkers.EventEnqueuer
	if loop != nil {
		enqueuer = loop
	}
	return &Supervisor{
		cfg:       cfg,
		workers:   workers,
		readiness: readiness,
		ingest:    ingestor,
		loop:      loop,
		background: []kernel.Worker{
			runtimeworkers.NewHeartbeatWorker(workers),
			runtimeworkers.NewProcessingRecoveryWorker(workers, enqueuer),
			runtimeworkers.NewDeliveryPollWorker(workers, sender),
			runtimeworkers.NewDelayedJobsWorker(workers, ingestor, enqueuer),
			runtimeworkers.NewCronWorker(workers, ingestor, enqueuer),
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *Supervisor) Start() {
	if s.loop != nil {
		s.loop.Start()
	}
	s.wg.Add(len(s.background))
	for _, worker := range s.background {
		go s.runWorker(worker)
	}
	if s.workers != nil {
		_ = s.workers.UpsertCronJobs(context.Background(), s.cfg.TenantID, s.cfg.CronJobs, time.Now().UTC())
	}
}

func (s *Supervisor) Stop() {
	s.cancel()
	if s.loop != nil {
		s.loop.Stop()
	}
	s.wg.Wait()
}

func (s *Supervisor) EventLoopAlive() bool {
	return s.loop != nil && s.loop.IsAlive()
}

func (s *Supervisor) ReadyState(ctx context.Context) (map[string]any, error) {
	queueDepth := 0
	if s.loop != nil {
		queueDepth = s.loop.InboundDepth()
	}
	state := map[string]any{
		"status":      "ready",
		"queue_depth": queueDepth,
		"time":        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := s.readiness.CheckReadWrite(ctx); err != nil {
		state["status"] = "not_ready"
		state["db_error"] = err.Error()
		return state, err
	}
	if s.loop == nil || !s.loop.IsAlive() {
		state["status"] = "not_ready"
		state["event_loop"] = "down"
		return state, fmt.Errorf("event loop down")
	}
	workers := s.workerHeartbeatNames()
	if s.cfg.Channels.Telegram.Enabled {
		workers = append(workers, "telegram_polling")
	}
	for _, worker := range workers {
		beatAt, ok, err := s.readiness.HeartbeatAt(ctx, worker)
		if err != nil {
			state[worker] = "error"
			continue
		}
		if !ok {
			state[worker] = "missing"
			continue
		}
		if time.Since(beatAt) > heartbeatFreshness {
			state[worker] = "stale"
		} else {
			state[worker] = "alive"
		}
	}
	return state, nil
}

func (s *Supervisor) runWorker(worker kernel.Worker) {
	defer s.wg.Done()
	_ = worker.Run(s.ctx)
}

func (s *Supervisor) workerHeartbeatNames() []string {
	names := make([]string, 0, len(s.background))
	for _, worker := range s.background {
		role := worker.Role()
		heartbeat := role.HeartbeatName
		if heartbeat == "" {
			heartbeat = role.Name
		}
		names = append(names, heartbeat)
	}
	return names
}

func (s *Supervisor) runScheduledKind(now time.Time, kind model.ScheduledJobKind) {
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	var enqueuer runtimeworkers.EventEnqueuer
	if s.loop != nil {
		enqueuer = s.loop
	}
	runtimeworkers.RunScheduledKind(ctx, s.workers, s.ingest, enqueuer, kind, now)
}
