package kernel

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type Service struct {
	facts    Facts
	executor Executor
	events   EventSink
	now      func() time.Time
	nextID   func() uint64
	counter  atomic.Uint64
}

func NewService(facts Facts, executor Executor, events EventSink) *Service {
	return &Service{
		facts:    facts,
		executor: executor,
		events:   events,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *Service) SetClock(now func() time.Time) {
	if now == nil {
		return
	}
	s.now = now
}

func (s *Service) SetIDGenerator(next func() uint64) {
	s.nextID = next
}

func (s *Service) Process(ctx context.Context, work runtimemodel.WorkItem) error {
	if s.facts == nil || s.executor == nil {
		return nil
	}
	claim, claimedAt, ok, err := s.claim(ctx, work)
	if err != nil || !ok {
		return err
	}
	logger := logging.L("runtime.kernel").With(
		logging.String("event_id", claim.Event.EventID),
		logging.String("session_key", claim.SessionKey),
		logging.String("session_id", claim.SessionID),
		logging.String("run_id", claim.RunID),
	)

	s.publish(ctx, runtimemodel.RuntimeEvent{
		Kind:       runtimemodel.RuntimeEventClaimed,
		Work:       claim.Work,
		EventID:    claim.Event.EventID,
		RunID:      claim.RunID,
		SessionKey: claim.SessionKey,
		SessionID:  claim.SessionID,
		Message:    "claimed",
		OccurredAt: claimedAt,
	})
	s.publish(ctx, runtimemodel.RuntimeEvent{
		Kind:       runtimemodel.RuntimeEventExecuting,
		Work:       claim.Work,
		EventID:    claim.Event.EventID,
		RunID:      claim.RunID,
		SessionKey: claim.SessionKey,
		SessionID:  claim.SessionID,
		Message:    "running",
		OccurredAt: claimedAt,
	})

	result, runErr := s.execute(ctx, claim)
	finalize := s.buildFinalize(claim, result, runErr)
	s.publish(ctx, runtimemodel.RuntimeEvent{
		Kind:       runtimemodel.RuntimeEventFinalizeStarted,
		Work:       claim.Work,
		EventID:    claim.Event.EventID,
		RunID:      claim.RunID,
		SessionKey: claim.SessionKey,
		SessionID:  claim.SessionID,
		Message:    "finalizing",
		OccurredAt: finalize.Now,
	})
	if err := s.facts.Finalize(ctx, finalize); err != nil {
		logger.Error("finalize failed",
			logging.String("status", "failed"),
			logging.String("error_code", model.ErrorCodeInternal),
			logging.Error(err),
		)
		s.publish(ctx, runtimemodel.RuntimeEvent{
			Kind:       runtimemodel.RuntimeEventFailed,
			Work:       claim.Work,
			EventID:    claim.Event.EventID,
			RunID:      claim.RunID,
			SessionKey: claim.SessionKey,
			SessionID:  claim.SessionID,
			Message:    "finalize failed",
			OccurredAt: s.now(),
			Error: &model.ErrorBlock{
				Code:    model.ErrorCodeInternal,
				Message: err.Error(),
			},
		})
		return err
	}

	eventKind := runtimemodel.RuntimeEventCompleted
	if finalize.EventStatus == model.EventStatusFailed {
		eventKind = runtimemodel.RuntimeEventFailed
	}
	s.publish(ctx, runtimemodel.RuntimeEvent{
		Kind:       eventKind,
		Work:       claim.Work,
		EventID:    claim.Event.EventID,
		RunID:      claim.RunID,
		SessionKey: claim.SessionKey,
		SessionID:  claim.SessionID,
		Message:    string(finalize.EventStatus),
		OccurredAt: finalize.Now,
		Error:      finalize.Error,
	})
	logger.Info("completed",
		logging.String("status", string(finalize.EventStatus)),
		logging.Int64("latency_ms", time.Since(claimedAt).Milliseconds()),
	)
	return nil
}

func (s *Service) execute(ctx context.Context, claim runtimemodel.ClaimContext) (result runtimemodel.ExecutionResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("runner panic: %v", recovered)
		}
		if result.RunMode == "" {
			result.RunMode = claim.RunMode
		}
	}()
	return s.executor.Execute(ctx, claim, s.events)
}

func (s *Service) publish(ctx context.Context, event runtimemodel.RuntimeEvent) {
	if s.events == nil {
		return
	}
	_ = s.events.Publish(ctx, event)
}

func (s *Service) next() uint64 {
	if s.nextID != nil {
		return s.nextID()
	}
	return s.counter.Add(1)
}

func (s *Service) messageID(now time.Time) string {
	return fmt.Sprintf("msg_%d_%d", now.UnixNano(), s.next())
}
