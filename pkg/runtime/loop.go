package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/bus"
	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/outbound"
	"github.com/similarityyoung/simiclaw/pkg/runner"
	"github.com/similarityyoung/simiclaw/pkg/store"
)

type EventLoop struct {
	bus       *bus.MessageBus
	repo      *EventRepo
	runner    runner.Runner
	storeLoop *store.StoreLoop
	outbound  *outbound.Hub
	maxRounds int
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func NewEventLoop(eventBus *bus.MessageBus, repo *EventRepo, runner runner.Runner, storeLoop *store.StoreLoop, outboundHub *outbound.Hub, maxRounds int) *EventLoop {
	ctx, cancel := context.WithCancel(context.Background())
	return &EventLoop{
		bus:       eventBus,
		repo:      repo,
		runner:    runner,
		storeLoop: storeLoop,
		outbound:  outboundHub,
		maxRounds: maxRounds,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (l *EventLoop) Start() {
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		for {
			evt, ok := l.bus.ConsumeInbound(l.ctx)
			if !ok {
				return
			}
			l.handle(evt)
		}
	}()
}

func (l *EventLoop) Stop() {
	l.cancel()
	l.wg.Wait()
}

func (l *EventLoop) StopAfterDrain() {
	l.wg.Wait()
	l.cancel()
}

func (l *EventLoop) handle(evt model.InternalEvent) {
	now := time.Now().UTC()
	_ = l.repo.Update(evt.EventID, func(rec *model.EventRecord) {
		rec.Status = model.EventStatusRunning
		rec.DeliveryStatus = model.DeliveryStatusNotApplicable
		rec.DeliveryDetail = model.DeliveryDetailNotApplicable
	})

	output, err := l.runner.Run(l.ctx, evt, l.maxRounds)
	if err != nil {
		_ = l.repo.Update(evt.EventID, func(rec *model.EventRecord) {
			rec.Status = model.EventStatusFailed
			rec.Error = &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: err.Error()}
		})
		return
	}

	output.Trace.EventID = evt.EventID
	output.Trace.SessionKey = evt.SessionKey
	output.Trace.SessionID = evt.ActiveSessionID
	output.Trace.RunMode = output.RunMode
	output.Trace.StartedAt = now
	output.Trace.FinishedAt = time.Now().UTC()

	commitID, err := l.storeLoop.Commit(l.ctx, store.CommitRequest{
		SessionKey: evt.SessionKey,
		SessionID:  evt.ActiveSessionID,
		Entries:    output.Entries,
		RunTrace:   output.Trace,
		Now:        time.Now().UTC(),
	})
	if err != nil {
		_ = l.repo.Update(evt.EventID, func(rec *model.EventRecord) {
			rec.Status = model.EventStatusFailed
			rec.Error = &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: fmt.Sprintf("commit failed: %v", err)}
		})
		return
	}

	_ = l.repo.Update(evt.EventID, func(rec *model.EventRecord) {
		rec.Status = model.EventStatusCommitted
		rec.RunID = output.RunID
		rec.RunMode = output.RunMode
		rec.CommitID = commitID
		rec.AssistantReply = output.OutboundBody
		rec.DeliveryStatus = model.DeliveryStatusPending
		rec.DeliveryDetail = model.DeliveryDetailDirect
	})

	if output.SuppressOutput {
		_ = l.repo.Update(evt.EventID, func(rec *model.EventRecord) {
			rec.DeliveryStatus = model.DeliveryStatusSuppressed
			rec.DeliveryDetail = model.DeliveryDetailNotApplicable
		})
		return
	}

	res := l.outbound.Send(l.ctx, evt.EventID, evt.SessionKey, output.OutboundBody)
	_ = l.repo.Update(evt.EventID, func(rec *model.EventRecord) {
		rec.DeliveryStatus = res.DeliveryStatus
		rec.DeliveryDetail = res.DeliveryDetail
		rec.OutboxID = res.OutboxID
		if res.Err != nil {
			rec.Error = &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: res.Err.Error()}
		}
	})
}
