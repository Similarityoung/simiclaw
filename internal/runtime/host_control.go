package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/internal/config"
	outbounddelivery "github.com/similarityyoung/simiclaw/internal/outbound/delivery"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	runtimeworkers "github.com/similarityyoung/simiclaw/internal/runtime/workers"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

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

type runtimeHost interface {
	runtimeworkers.EventEnqueuer
	Start(ctx context.Context) error
	Stop()
	IsAlive() bool
	InboundDepth() int
}

type HostControl struct {
	cfg     config.Config
	workers WorkerRepository
	ingest  runtimeworkers.EventIngestor
	host    *workerHost
	logger  *logging.Logger
}

type workerHost struct {
	loop       runtimeHost
	background []kernel.Worker
	logger     *logging.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func NewHostControl(cfg config.Config, workers WorkerRepository, ingestor runtimeworkers.EventIngestor, loop runtimeHost, sender outbounddelivery.Sender) *HostControl {
	logger := logging.L("runtime.host")
	return &HostControl{
		cfg:     cfg,
		workers: workers,
		ingest:  ingestor,
		host:    newWorkerHost(loop, buildBackgroundWorkers(workers, ingestor, loop, sender), logger),
		logger:  logger,
	}
}

func newWorkerHost(loop runtimeHost, background []kernel.Worker, logger *logging.Logger) *workerHost {
	if logger == nil {
		logger = logging.L("runtime.host")
	}
	return &workerHost{
		loop:       loop,
		background: background,
		logger:     logger,
	}
}

func buildBackgroundWorkers(workers WorkerRepository, ingestor runtimeworkers.EventIngestor, queue runtimeworkers.EventEnqueuer, sender outbounddelivery.Sender) []kernel.Worker {
	registry := runtimeworkers.NewRegistry()
	runtimeworkers.RegisterBuiltins(registry, runtimeworkers.Builtins{
		Heartbeat:  workers,
		Processing: workers,
		Scheduled:  workers,
		Ingest:     ingestor,
		Queue:      queue,
	})
	registry.Register(outbounddelivery.NewWorker(workers, sender))
	return registry.All()
}

func (c *HostControl) Start(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("runtime host requires a non-nil context")
	}
	c.logger.Info("host starting", logging.Int("worker_count", c.host.WorkerCount()))
	if err := c.host.Start(ctx); err != nil {
		return err
	}
	if c.workers != nil {
		if err := c.workers.UpsertCronJobs(c.host.ctx, c.cfg.TenantID, c.cfg.CronJobs, time.Now().UTC()); err != nil {
			c.logger.Warn("cron job sync failed", logging.Error(err))
		} else if len(c.cfg.CronJobs) > 0 {
			c.logger.Info("cron jobs synced", logging.Int("count", len(c.cfg.CronJobs)))
		}
	}
	c.logger.Info("host started", logging.Int("worker_count", c.host.WorkerCount()))
	return nil
}

func (c *HostControl) Stop() {
	c.logger.Info("host stopping", logging.Int("worker_count", c.host.WorkerCount()))
	c.host.Stop()
	c.logger.Info("host stopped")
}

func (c *HostControl) Alive() bool {
	if c == nil {
		return false
	}
	return c.host.Alive()
}

func (c *HostControl) InboundDepth() int {
	if c == nil {
		return 0
	}
	return c.host.InboundDepth()
}

func (c *HostControl) WorkerHeartbeatNames() []string {
	if c == nil {
		return nil
	}
	return c.host.WorkerHeartbeatNames()
}

func (c *HostControl) runScheduledKind(ctx context.Context, now time.Time, kind model.ScheduledJobKind) {
	runtimeworkers.RunScheduledKind(ctx, c.workers, c.ingest, c.host.Enqueuer(), kind, now)
}

func (h *workerHost) Start(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("runtime host requires a non-nil context")
	}
	h.ctx, h.cancel = context.WithCancel(ctx)
	if h.loop != nil {
		if err := h.loop.Start(h.ctx); err != nil {
			h.logger.Error("event loop start failed", logging.Error(err))
			h.cancel()
			h.ctx = nil
			h.cancel = nil
			return err
		}
	}
	h.wg.Add(len(h.background))
	for _, worker := range h.background {
		role := worker.Role()
		h.logger.Info(
			"worker starting",
			logging.String("worker", role.Name),
			logging.String("heartbeat", role.HeartbeatName),
		)
		go h.runWorker(worker)
	}
	return nil
}

func (h *workerHost) Stop() {
	if h.cancel != nil {
		h.cancel()
	}
	if h.loop != nil {
		h.loop.Stop()
	}
	h.wg.Wait()
}

func (h *workerHost) Alive() bool {
	return h != nil && h.loop != nil && h.loop.IsAlive()
}

func (h *workerHost) InboundDepth() int {
	if h == nil || h.loop == nil {
		return 0
	}
	return h.loop.InboundDepth()
}

func (h *workerHost) WorkerCount() int {
	if h == nil {
		return 0
	}
	return len(h.background)
}

func (h *workerHost) WorkerHeartbeatNames() []string {
	if h == nil {
		return nil
	}
	names := make([]string, 0, len(h.background))
	for _, worker := range h.background {
		role := worker.Role()
		heartbeat := role.HeartbeatName
		if heartbeat == "" {
			heartbeat = role.Name
		}
		names = append(names, heartbeat)
	}
	return names
}

func (h *workerHost) Enqueuer() runtimeworkers.EventEnqueuer {
	if h == nil {
		return nil
	}
	return h.loop
}

func (h *workerHost) runWorker(worker kernel.Worker) {
	defer h.wg.Done()
	role := worker.Role()
	logger := h.logger.With(
		logging.String("worker", role.Name),
		logging.String("heartbeat", role.HeartbeatName),
	)
	if err := worker.Run(h.ctx); err != nil {
		logger.Error("worker stopped with error", logging.Error(err))
		return
	}
	logger.Info("worker stopped")
}
