package runtime

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/similarityyoung/simiclaw/internal/ingest"
	"github.com/similarityyoung/simiclaw/internal/outbound"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const (
	heartbeatInterval   = 10 * time.Second
	heartbeatFreshness  = 30 * time.Second
	processingLease     = 2 * time.Minute
	processingSweepTick = 10 * time.Second
	outboxSendingLease  = 30 * time.Second
	outboxPollTick      = 500 * time.Millisecond
	delayedPollTick     = time.Second
)

type Supervisor struct {
	cfg     config.Config
	db      *store.DB
	ingest  EventIngestor
	loop    *EventLoop
	sender  outbound.Sender
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	healthy atomic.Bool
}

type EventIngestor interface {
	Ingest(ctx context.Context, cmd ingest.Command) (ingest.Result, *ingest.Error)
}

func NewSupervisor(cfg config.Config, db *store.DB, ingestor EventIngestor, loop *EventLoop, sender outbound.Sender) *Supervisor {
	ctx, cancel := context.WithCancel(context.Background())
	return &Supervisor{
		cfg:    cfg,
		db:     db,
		ingest: ingestor,
		loop:   loop,
		sender: sender,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *Supervisor) Start() {
	s.loop.Start()
	s.healthy.Store(true)
	s.wg.Add(5)
	go s.heartbeatWorker()
	go s.processingSweeper()
	go s.outboxWorker()
	go s.delayedJobWorker()
	go s.cronWorker()
	_ = s.db.UpsertCronJobs(context.Background(), s.cfg.TenantID, s.cfg.CronJobs, time.Now().UTC())
}

func (s *Supervisor) Stop() {
	s.cancel()
	s.loop.Stop()
	s.wg.Wait()
	s.healthy.Store(false)
}

func (s *Supervisor) EventLoopAlive() bool {
	return s.loop.IsAlive()
}

func (s *Supervisor) ReadyState(ctx context.Context) (map[string]any, error) {
	state := map[string]any{
		"status":      "ready",
		"queue_depth": s.loop.InboundDepth(),
		"time":        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := s.db.CheckReadWrite(ctx); err != nil {
		state["status"] = "not_ready"
		state["db_error"] = err.Error()
		return state, err
	}
	if !s.loop.IsAlive() {
		state["status"] = "not_ready"
		state["event_loop"] = "down"
		return state, fmt.Errorf("event loop down")
	}
	workers := []string{"heartbeat", "processing_sweeper", "outbox_retry", "delayed_jobs", "cron"}
	if s.cfg.Channels.Telegram.Enabled {
		workers = append(workers, "telegram_polling")
	}
	for _, worker := range workers {
		beatAt, ok, err := s.db.HeartbeatAt(ctx, worker)
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

func (s *Supervisor) heartbeatWorker() {
	defer s.wg.Done()
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			_ = s.db.BeatHeartbeat(s.ctx, "heartbeat", time.Now().UTC())
		}
	}
}

func (s *Supervisor) processingSweeper() {
	defer s.wg.Done()
	ticker := time.NewTicker(processingSweepTick)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().UTC()
			_ = s.db.BeatHeartbeat(s.ctx, "processing_sweeper", now)
			ids, err := s.db.RecoverExpiredProcessing(s.ctx, now.Add(-processingLease), now)
			if err != nil {
				continue
			}
			for _, id := range ids {
				s.loop.TryEnqueue(id)
			}
		}
	}
}

func (s *Supervisor) outboxWorker() {
	defer s.wg.Done()
	ticker := time.NewTicker(outboxPollTick)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().UTC()
			_ = s.db.BeatHeartbeat(s.ctx, "outbox_retry", now)
			_ = s.db.RecoverExpiredSending(s.ctx, now.Add(-outboxSendingLease), now)
			msg, ok, err := s.db.ClaimOutbox(s.ctx, "outbox-worker", now)
			if err != nil || !ok {
				continue
			}
			if err := s.sender.Send(s.ctx, model.OutboxMessage{
				OutboxID:   msg.OutboxID,
				EventID:    msg.EventID,
				SessionKey: msg.SessionKey,
				Channel:    msg.Channel,
				TargetID:   msg.TargetID,
				Body:       msg.Body,
				CreatedAt:  msg.CreatedAt,
				Attempts:   msg.AttemptCount,
			}); err != nil {
				backoff := time.Duration(msg.AttemptCount+1) * 5 * time.Second
				if backoff > 5*time.Minute {
					backoff = 5 * time.Minute
				}
				dead := msg.AttemptCount >= 5
				_ = s.db.FailOutboxSend(s.ctx, msg.OutboxID, msg.EventID, err.Error(), dead, now.Add(backoff), now)
				continue
			}
			_ = s.db.CompleteOutboxSend(s.ctx, msg.OutboxID, msg.EventID, now)
		}
	}
}

func (s *Supervisor) delayedJobWorker() {
	defer s.wg.Done()
	ticker := time.NewTicker(delayedPollTick)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().UTC()
			_ = s.db.BeatHeartbeat(s.ctx, "delayed_jobs", now)
			s.runScheduledKind(now, model.ScheduledJobKindDelayed)
			s.runScheduledKind(now, model.ScheduledJobKindRetry)
		}
	}
}

func (s *Supervisor) cronWorker() {
	defer s.wg.Done()
	ticker := time.NewTicker(delayedPollTick)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().UTC()
			_ = s.db.BeatHeartbeat(s.ctx, "cron", now)
			s.runScheduledKind(now, model.ScheduledJobKindCron)
		}
	}
}

func (s *Supervisor) runScheduledKind(now time.Time, kind model.ScheduledJobKind) {
	job, ok, err := s.db.ClaimScheduledJob(s.ctx, kind, string(kind)+"-worker", now)
	if err != nil || !ok {
		return
	}
	req := model.IngestRequest{
		Source:         job.Payload.Source,
		Conversation:   job.Payload.Conversation,
		IdempotencyKey: fmt.Sprintf("%s:%d", job.JobID, now.Unix()),
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        job.Payload.Payload,
	}
	if s.ingest == nil {
		_ = s.db.FailScheduledJob(s.ctx, job.JobID, "ingest service unavailable", now.Add(30*time.Second), now)
		return
	}
	result, ingestErr := s.ingest.Ingest(s.ctx, ingest.Command{Request: req, ReceivedAt: now})
	if ingestErr != nil {
		_ = s.db.FailScheduledJob(s.ctx, job.JobID, ingestErr.Error(), now.Add(30*time.Second), now)
		return
	}
	_ = s.db.CompleteScheduledJob(s.ctx, job, now)
	if !result.Duplicate && !result.Enqueued && s.loop != nil {
		s.loop.TryEnqueue(result.EventID)
	}
	_ = s.db.BeatHeartbeat(s.ctx, string(kind), now)
}
