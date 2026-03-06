package runtime

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/runner"
	"github.com/similarityyoung/simiclaw/pkg/store"
)

type EventLoop struct {
	db        *store.DB
	runner    runner.Runner
	maxRounds int
	queue     chan string
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	alive     atomic.Bool
	enqueueID atomic.Uint64
}

func NewEventLoop(db *store.DB, run runner.Runner, queueCap, maxRounds int) *EventLoop {
	if queueCap <= 0 {
		queueCap = 1024
	}
	if maxRounds <= 0 {
		maxRounds = 4
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &EventLoop{
		db:        db,
		runner:    run,
		maxRounds: maxRounds,
		queue:     make(chan string, queueCap),
		ctx:       ctx,
		cancel:    cancel,
	}
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
			ids, err := l.db.ListRunnableEventIDs(ctx, cap(l.queue))
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
	now := time.Now().UTC()
	runID := fmt.Sprintf("run_%d_%d", now.UnixNano(), l.enqueueID.Add(1))
	ctx, cancel := context.WithTimeout(l.ctx, 2*time.Minute)
	defer cancel()

	claimed, ok, err := l.db.ClaimEvent(ctx, eventID, runID, now)
	if err != nil || !ok {
		return
	}

	logger := logging.L("eventloop").With(
		logging.String("event_id", claimed.Event.EventID),
		logging.String("session_key", claimed.Event.SessionKey),
		logging.String("session_id", claimed.Event.ActiveSessionID),
		logging.String("run_id", claimed.RunID),
	)
	output, runErr := l.runner.Run(ctx, claimed.Event, l.maxRounds)
	finalize := store.RunFinalize{
		RunID:       claimed.RunID,
		EventID:     claimed.Event.EventID,
		SessionKey:  claimed.Event.SessionKey,
		SessionID:   claimed.Event.ActiveSessionID,
		RunMode:     output.RunMode,
		RunStatus:   model.RunStatusCompleted,
		EventStatus: model.EventStatusProcessed,
		Now:         time.Now().UTC(),
	}
	if output.Trace.Provider != "" {
		finalize.Provider = output.Trace.Provider
		finalize.Model = output.Trace.Model
		finalize.PromptTokens = output.Trace.PromptTokens
		finalize.CompletionTokens = output.Trace.CompletionTokens
		finalize.TotalTokens = output.Trace.TotalTokens
		finalize.LatencyMS = output.Trace.LatencyMS
		finalize.FinishReason = output.Trace.FinishReason
		finalize.RawFinishReason = output.Trace.RawFinishReason
		finalize.ProviderRequestID = output.Trace.ProviderRequestID
		finalize.OutputText = output.Trace.OutputText
		finalize.ToolCalls = output.Trace.ToolCalls
		finalize.Diagnostics = output.Trace.Diagnostics
	}
	for _, msg := range output.Messages {
		finalize.Messages = append(finalize.Messages, store.StoredMessage{
			MessageID:  fmt.Sprintf("msg_%d_%d", finalize.Now.UnixNano(), l.enqueueID.Add(1)),
			SessionKey: claimed.Event.SessionKey,
			SessionID:  claimed.Event.ActiveSessionID,
			RunID:      claimed.RunID,
			Role:       msg.Role,
			Content:    msg.Content,
			Visible:    msg.Visible,
			ToolCallID: msg.ToolCallID,
			ToolName:   msg.ToolName,
			ToolArgs:   msg.ToolArgs,
			ToolResult: msg.ToolResult,
			Meta:       msg.Meta,
			CreatedAt:  finalize.Now,
		})
	}
	if runErr != nil {
		finalize.RunStatus = model.RunStatusFailed
		finalize.EventStatus = model.EventStatusFailed
		finalize.Error = &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: runErr.Error()}
	} else if output.SuppressOutput {
		finalize.EventStatus = model.EventStatusSuppressed
		finalize.AssistantReply = ""
	} else {
		finalize.AssistantReply = output.AssistantReply
		finalize.OutboxBody = output.AssistantReply
	}
	if err := l.db.FinalizeRun(ctx, finalize); err != nil {
		logger.Error("eventloop.finalize_failed",
			logging.String("status", "failed"),
			logging.String("error_code", model.ErrorCodeInternal),
			logging.Error(err),
		)
		return
	}
	logger.Info("eventloop.completed",
		logging.String("status", string(finalize.EventStatus)),
		logging.Int64("latency_ms", time.Since(now).Milliseconds()),
	)
}
