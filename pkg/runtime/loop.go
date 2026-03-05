package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/approval"
	"github.com/similarityyoung/simiclaw/pkg/bus"
	"github.com/similarityyoung/simiclaw/pkg/logging"
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
	approvals *approval.Service
	maxRounds int
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func NewEventLoop(eventBus *bus.MessageBus, repo *EventRepo, runner runner.Runner, storeLoop *store.StoreLoop, outboundHub *outbound.Hub, approvalSvc *approval.Service, maxRounds int) *EventLoop {
	ctx, cancel := context.WithCancel(context.Background())
	return &EventLoop{
		bus:       eventBus,
		repo:      repo,
		runner:    runner,
		storeLoop: storeLoop,
		outbound:  outboundHub,
		approvals: approvalSvc,
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
	start := time.Now()
	now := start.UTC()
	logger := logging.L("eventloop").With(
		logging.String("event_id", evt.EventID),
		logging.String("tenant_id", evt.TenantID),
		logging.String("conversation_id", evt.Conversation.ConversationID),
		logging.String("session_key", evt.SessionKey),
		logging.String("session_id", evt.ActiveSessionID),
	)

	l.ensureEventRecord(evt, now)
	_ = l.repo.Update(evt.EventID, func(rec *model.EventRecord) {
		rec.Status = model.EventStatusRunning
		rec.DeliveryStatus = model.DeliveryStatusNotApplicable
		rec.DeliveryDetail = model.DeliveryDetailNotApplicable
	})

	var (
		output runner.RunOutput
		err    error
	)
	if isApprovalPayload(evt.Payload.Type) {
		output, err = l.buildApprovalRunOutput(evt, now)
	} else {
		output, err = l.runner.Run(l.ctx, evt, l.maxRounds)
		if err == nil {
			highRisk := selectHighRiskActions(output.Trace.Actions)
			if len(highRisk) > 0 && l.approvals != nil {
				approvalRec, createErr := l.approvals.CreateAuto(
					output.RunID,
					evt.SessionKey,
					evt.ActiveSessionID,
					evt.Conversation.ConversationID,
					evt.Scopes,
					highRisk,
					"检测到高风险动作，等待审批",
					now,
				)
				if createErr != nil {
					err = createErr
				} else {
					output.Trace.Actions = append(output.Trace.Actions, model.Action{
						ActionID:             fmt.Sprintf("act_req_%d", now.UnixNano()),
						ActionIndex:          len(output.Trace.Actions),
						ActionIdempotencyKey: fmt.Sprintf("%s:%d", output.RunID, len(output.Trace.Actions)),
						Type:                 "RequestApproval",
						Risk:                 "low",
						RequiresApproval:     false,
						Payload: map[string]any{
							"approval_id": approvalRec.ApprovalID,
						},
					})
					msg := fmt.Sprintf("高风险动作已进入审批队列：%s", approvalRec.ApprovalID)
					output.Entries = append(output.Entries, model.SessionEntry{
						Type:    "assistant",
						EntryID: fmt.Sprintf("e_assistant_%d", now.UnixNano()),
						RunID:   output.RunID,
						Content: msg,
					})
					output.OutboundBody = msg
				}
			}
		}
	}
	if err != nil {
		_ = l.repo.Update(evt.EventID, func(rec *model.EventRecord) {
			rec.Status = model.EventStatusFailed
			rec.Error = &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: err.Error()}
		})
		logger.Error("eventloop.run_failed",
			logging.String("status", "failed"),
			logging.String("error_code", model.ErrorCodeInternal),
			logging.Error(err),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
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
		logger.Error("eventloop.commit_failed",
			logging.String("status", "failed"),
			logging.String("run_id", output.RunID),
			logging.String("run_mode", string(output.RunMode)),
			logging.String("error_code", model.ErrorCodeInternal),
			logging.Error(err),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
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
	logger.Info("eventloop.committed",
		logging.String("status", string(model.EventStatusCommitted)),
		logging.String("run_id", output.RunID),
		logging.String("run_mode", string(output.RunMode)),
		logging.String("commit_id", commitID),
		logging.Int64("latency_ms", time.Since(start).Milliseconds()),
	)

	if output.SuppressOutput {
		_ = l.repo.Update(evt.EventID, func(rec *model.EventRecord) {
			rec.DeliveryStatus = model.DeliveryStatusSuppressed
			rec.DeliveryDetail = model.DeliveryDetailNotApplicable
		})
		logger.Info("eventloop.outbound.result",
			logging.String("status", string(model.EventStatusCommitted)),
			logging.String("run_id", output.RunID),
			logging.String("run_mode", string(output.RunMode)),
			logging.String("commit_id", commitID),
			logging.String("delivery_status", string(model.DeliveryStatusSuppressed)),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
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
	if res.Err != nil {
		logger.Warn("eventloop.outbound.result",
			logging.String("status", string(model.EventStatusCommitted)),
			logging.String("run_id", output.RunID),
			logging.String("run_mode", string(output.RunMode)),
			logging.String("commit_id", commitID),
			logging.String("delivery_status", string(res.DeliveryStatus)),
			logging.String("error_code", model.ErrorCodeInternal),
			logging.Error(res.Err),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		return
	}
	logger.Info("eventloop.outbound.result",
		logging.String("status", string(model.EventStatusCommitted)),
		logging.String("run_id", output.RunID),
		logging.String("run_mode", string(output.RunMode)),
		logging.String("commit_id", commitID),
		logging.String("delivery_status", string(res.DeliveryStatus)),
		logging.Int64("latency_ms", time.Since(start).Milliseconds()),
	)
}

func (l *EventLoop) ensureEventRecord(evt model.InternalEvent, now time.Time) {
	if _, ok := l.repo.Get(evt.EventID); ok {
		return
	}
	_ = l.repo.Put(model.EventRecord{
		EventID:        evt.EventID,
		Status:         model.EventStatusAccepted,
		DeliveryStatus: model.DeliveryStatusNotApplicable,
		DeliveryDetail: model.DeliveryDetailNotApplicable,
		SessionKey:     evt.SessionKey,
		SessionID:      evt.ActiveSessionID,
		RunMode:        model.RunModeNormal,
		ReceivedAt:     now,
		UpdatedAt:      now,
		PayloadHash:    evt.IdempotencyKey,
	})
}

func isApprovalPayload(payloadType string) bool {
	return payloadType == "approval_granted" || payloadType == "approval_rejected"
}

func selectHighRiskActions(actions []model.Action) []model.Action {
	out := make([]model.Action, 0, len(actions))
	for _, action := range actions {
		if action.RequiresApproval || strings.EqualFold(action.Risk, "high") {
			out = append(out, action)
		}
	}
	return out
}

func (l *EventLoop) buildApprovalRunOutput(evt model.InternalEvent, now time.Time) (runner.RunOutput, error) {
	runID := fmt.Sprintf("run_%d", now.UnixNano())
	approvalID := strings.TrimSpace(evt.Payload.ApprovalID)
	if approvalID == "" {
		return runner.RunOutput{}, fmt.Errorf("approval_id is required")
	}

	entries := []model.SessionEntry{{
		Type:    "system",
		EntryID: fmt.Sprintf("e_system_%d", now.UnixNano()),
		RunID:   runID,
		Content: fmt.Sprintf("approval event: %s", evt.Payload.Type),
	}}
	actions := make([]model.Action, 0, 4)
	reply := ""
	switch evt.Payload.Type {
	case "approval_granted":
		if l.approvals == nil {
			return runner.RunOutput{}, fmt.Errorf("approval service not configured")
		}
		summary, results, err := l.approvals.ExecuteApproved(approvalID, now)
		if err != nil {
			return runner.RunOutput{}, err
		}
		reply = fmt.Sprintf("%s（%s）", summary, approvalID)
		for _, result := range results {
			actions = append(actions, model.Action{
				ActionID:             result.ActionID,
				ActionIndex:          len(actions),
				ActionIdempotencyKey: fmt.Sprintf("%s:%d", runID, len(actions)),
				Type:                 "Patch",
				Risk:                 "high",
				RequiresApproval:     false,
				Payload: map[string]any{
					"ok":      result.OK,
					"message": result.Message,
				},
			})
		}
	case "approval_rejected":
		reply = fmt.Sprintf("审批已拒绝，未执行变更。（%s）", approvalID)
	default:
		return runner.RunOutput{}, fmt.Errorf("unsupported approval payload type: %s", evt.Payload.Type)
	}
	entries = append(entries, model.SessionEntry{
		Type:    "assistant",
		EntryID: fmt.Sprintf("e_assistant_%d", now.UnixNano()),
		RunID:   runID,
		Content: reply,
	})
	trace := model.RunTrace{
		RunID:       runID,
		EventID:     evt.EventID,
		SessionKey:  evt.SessionKey,
		SessionID:   evt.ActiveSessionID,
		RunMode:     model.RunModeNormal,
		Actions:     actions,
		StartedAt:   now,
		FinishedAt:  now,
		Diagnostics: map[string]string{"approval_id": approvalID},
	}
	return runner.RunOutput{
		RunID:          runID,
		RunMode:        model.RunModeNormal,
		Entries:        entries,
		Trace:          trace,
		OutboundBody:   reply,
		SuppressOutput: false,
	}, nil
}
