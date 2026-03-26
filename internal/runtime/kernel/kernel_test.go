package kernel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/internal/testutil/logcapture"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestServiceProcessPublishesLifecycleAndFinalizesSuccess(t *testing.T) {
	now := time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC)
	work := runtimemodel.WorkItem{EventID: "evt_success"}
	facts := &stubFacts{
		claimOK: true,
		claimCtx: runtimemodel.ClaimContext{
			Work:       work,
			Event:      model.InternalEvent{EventID: "evt_success", SessionKey: "session:success", ActiveSessionID: "ses_success"},
			RunMode:    model.RunModeNormal,
			SessionKey: "session:success",
			SessionID:  "ses_success",
		},
	}
	executor := &stubExecutor{
		result: runtimemodel.ExecutionResult{
			RunMode:        model.RunModeNormal,
			AssistantReply: "hello",
			OutputMessages: []runtimemodel.StoredMessage{
				{Role: "assistant", Content: "hello", Visible: true},
			},
			Delivery: &runtimemodel.DeliveryIntent{
				Channel:  "telegram",
				TargetID: "42",
				Body:     "hello",
			},
		},
	}
	sink := &captureEventSink{}
	svc := NewService(facts, nil, executor, sink)
	svc.SetClock(func() time.Time { return now })
	var nextID uint64
	svc.SetIDGenerator(func() uint64 {
		nextID++
		return nextID
	})

	if err := svc.Process(context.Background(), work); err != nil {
		t.Fatalf("Process: %v", err)
	}

	if got := facts.claimCalls; got != 1 {
		t.Fatalf("expected ClaimWork once, got %d", got)
	}
	if got := executor.calls; got != 1 {
		t.Fatalf("expected Execute once, got %d", got)
	}
	if len(facts.finalizeCmds) != 1 {
		t.Fatalf("expected one finalize command, got %d", len(facts.finalizeCmds))
	}
	finalize := facts.finalizeCmds[0]
	wantRunID := fmt.Sprintf("run_%d_1", now.UnixNano())
	if finalize.RunID != wantRunID {
		t.Fatalf("expected run id %q, got %q", wantRunID, finalize.RunID)
	}
	if finalize.RunStatus != model.RunStatusCompleted || finalize.EventStatus != model.EventStatusProcessed {
		t.Fatalf("expected successful finalize, got %+v", finalize)
	}
	if finalize.AssistantReply != "hello" {
		t.Fatalf("expected assistant reply to be preserved, got %+v", finalize)
	}
	if finalize.OutboxChannel != "telegram" || finalize.OutboxTargetID != "42" || finalize.OutboxBody != "hello" {
		t.Fatalf("expected delivery intent copied into finalize, got %+v", finalize)
	}
	if len(finalize.Messages) != 1 {
		t.Fatalf("expected one stored message, got %+v", finalize.Messages)
	}
	msg := finalize.Messages[0]
	if msg.MessageID == "" || msg.RunID != wantRunID || msg.SessionKey != "session:success" || msg.SessionID != "ses_success" {
		t.Fatalf("expected stored message metadata to be populated, got %+v", msg)
	}
	assertEventKinds(t, sink.events, []runtimemodel.RuntimeEventKind{
		runtimemodel.RuntimeEventClaimed,
		runtimemodel.RuntimeEventExecuting,
		runtimemodel.RuntimeEventFinalizeStarted,
		runtimemodel.RuntimeEventCompleted,
	})
}

func TestServiceProcessStopsWhenClaimRejected(t *testing.T) {
	facts := &stubFacts{claimOK: false}
	executor := &stubExecutor{}
	sink := &captureEventSink{}
	svc := NewService(facts, nil, executor, sink)

	if err := svc.Process(context.Background(), runtimemodel.WorkItem{EventID: "evt_skip"}); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got := executor.calls; got != 0 {
		t.Fatalf("expected executor to stay idle, got %d calls", got)
	}
	if len(facts.finalizeCmds) != 0 {
		t.Fatalf("expected no finalize command, got %+v", facts.finalizeCmds)
	}
	if len(sink.events) != 0 {
		t.Fatalf("expected no published events, got %+v", sink.events)
	}
}

func TestServiceProcessRecoversExecutorPanicAsFailedFinalize(t *testing.T) {
	now := time.Date(2026, 3, 18, 10, 1, 0, 0, time.UTC)
	work := runtimemodel.WorkItem{EventID: "evt_panic"}
	facts := &stubFacts{
		claimOK: true,
		claimCtx: runtimemodel.ClaimContext{
			Work:       work,
			Event:      model.InternalEvent{EventID: "evt_panic", SessionKey: "session:panic", ActiveSessionID: "ses_panic"},
			RunMode:    model.RunModeNormal,
			SessionKey: "session:panic",
			SessionID:  "ses_panic",
		},
	}
	sink := &captureEventSink{}
	svc := NewService(facts, nil, &stubExecutor{panicValue: "boom"}, sink)
	svc.SetClock(func() time.Time { return now })

	if err := svc.Process(context.Background(), work); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(facts.finalizeCmds) != 1 {
		t.Fatalf("expected one finalize command, got %d", len(facts.finalizeCmds))
	}
	finalize := facts.finalizeCmds[0]
	if finalize.RunStatus != model.RunStatusFailed || finalize.EventStatus != model.EventStatusFailed {
		t.Fatalf("expected failed finalize after panic, got %+v", finalize)
	}
	if finalize.Error == nil || finalize.Error.Message != "runner panic: boom" {
		t.Fatalf("expected panic to become terminal error, got %+v", finalize.Error)
	}
	assertEventKinds(t, sink.events, []runtimemodel.RuntimeEventKind{
		runtimemodel.RuntimeEventClaimed,
		runtimemodel.RuntimeEventExecuting,
		runtimemodel.RuntimeEventFinalizeStarted,
		runtimemodel.RuntimeEventFailed,
	})
}

func TestServiceProcessReturnsFinalizeError(t *testing.T) {
	now := time.Date(2026, 3, 18, 10, 2, 0, 0, time.UTC)
	work := runtimemodel.WorkItem{EventID: "evt_finalize_fail"}
	facts := &stubFacts{
		claimOK: true,
		claimCtx: runtimemodel.ClaimContext{
			Work:       work,
			Event:      model.InternalEvent{EventID: "evt_finalize_fail", SessionKey: "session:finalize", ActiveSessionID: "ses_finalize"},
			RunMode:    model.RunModeNormal,
			SessionKey: "session:finalize",
			SessionID:  "ses_finalize",
		},
		finalizeErr: errors.New("finalize write failed"),
	}
	sink := &captureEventSink{}
	svc := NewService(facts, nil, &stubExecutor{result: runtimemodel.ExecutionResult{RunMode: model.RunModeNormal}}, sink)
	svc.SetClock(func() time.Time { return now })

	err := svc.Process(context.Background(), work)
	if err == nil || err.Error() != "finalize write failed" {
		t.Fatalf("expected finalize error, got %v", err)
	}
	if len(facts.finalizeCmds) != 1 {
		t.Fatalf("expected one finalize command, got %d", len(facts.finalizeCmds))
	}
	if len(sink.events) != 4 {
		t.Fatalf("expected lifecycle events plus terminal failure, got %+v", sink.events)
	}
	last := sink.events[len(sink.events)-1]
	if last.Kind != runtimemodel.RuntimeEventFailed {
		t.Fatalf("expected terminal failed event, got %+v", last)
	}
	if last.Error == nil || last.Error.Message != "finalize write failed" {
		t.Fatalf("expected finalize error attached to terminal event, got %+v", last.Error)
	}
}

func TestServiceProcessLogsLifecycleMilestones(t *testing.T) {
	now := time.Date(2026, 3, 18, 10, 3, 0, 0, time.UTC)
	work := runtimemodel.WorkItem{EventID: "evt_log"}
	facts := &stubFacts{
		claimOK: true,
		claimCtx: runtimemodel.ClaimContext{
			Work:       work,
			Event:      model.InternalEvent{EventID: "evt_log", SessionKey: "session:log", ActiveSessionID: "ses_log", Payload: model.EventPayload{Type: "message"}},
			RunMode:    model.RunModeNormal,
			SessionKey: "session:log",
			SessionID:  "ses_log",
		},
	}
	svc := NewService(facts, nil, &stubExecutor{result: runtimemodel.ExecutionResult{RunMode: model.RunModeNormal}}, NopEventSink{})
	svc.SetClock(func() time.Time { return now })

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		if err := svc.Process(context.Background(), work); err != nil {
			t.Fatalf("Process: %v", err)
		}
		_ = logging.Sync()
	})

	assertOutputContainsInOrder(t, out,
		"[runtime.kernel] claim succeeded",
		"[runtime.kernel] execution started",
		"[runtime.kernel] finalize started",
		"[runtime.kernel] completed",
	)
	for _, part := range []string{
		`"event_id": "evt_log"`,
		`"run_mode": "NORMAL"`,
		`"status": "processed"`,
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("missing %q in %q", part, out)
		}
	}
}

type stubFacts struct {
	mu           sync.Mutex
	claimCtx     runtimemodel.ClaimContext
	claimOK      bool
	claimErr     error
	finalizeErr  error
	claimCalls   int
	claimRunIDs  []string
	claimTimes   []time.Time
	finalizeCmds []runtimemodel.FinalizeCommand
}

func (f *stubFacts) ListRunnable(context.Context, int) ([]runtimemodel.WorkItem, error) {
	return nil, nil
}

func (f *stubFacts) ClaimWork(_ context.Context, work runtimemodel.WorkItem, runID string, now time.Time) (runtimemodel.ClaimContext, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimCalls++
	f.claimRunIDs = append(f.claimRunIDs, runID)
	f.claimTimes = append(f.claimTimes, now)

	claim := f.claimCtx
	if claim.RunID == "" {
		claim.RunID = runID
	}
	if claim.Work.EventID == "" {
		claim.Work = work
	}
	return claim, f.claimOK, f.claimErr
}

func (f *stubFacts) Finalize(_ context.Context, cmd runtimemodel.FinalizeCommand) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finalizeCmds = append(f.finalizeCmds, cmd)
	return f.finalizeErr
}

type stubExecutor struct {
	mu         sync.Mutex
	result     runtimemodel.ExecutionResult
	err        error
	panicValue any
	calls      int
}

func (e *stubExecutor) Execute(context.Context, runtimemodel.ClaimContext, EventSink) (runtimemodel.ExecutionResult, error) {
	if e.panicValue != nil {
		panic(e.panicValue)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	return e.result, e.err
}

type captureEventSink struct {
	mu     sync.Mutex
	events []runtimemodel.RuntimeEvent
}

func (s *captureEventSink) Publish(_ context.Context, event runtimemodel.RuntimeEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func assertOutputContainsInOrder(t *testing.T, out string, parts ...string) {
	t.Helper()

	last := -1
	for _, part := range parts {
		idx := strings.Index(out, part)
		if idx < 0 {
			t.Fatalf("missing %q in %q", part, out)
		}
		if idx <= last {
			t.Fatalf("out of order %q in %q", part, out)
		}
		last = idx
	}
}

func assertEventKinds(t *testing.T, events []runtimemodel.RuntimeEvent, want []runtimemodel.RuntimeEventKind) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("expected %d events, got %+v", len(want), events)
	}
	for i, kind := range want {
		if events[i].Kind != kind {
			t.Fatalf("expected event %d to be %q, got %+v", i, kind, events[i])
		}
	}
}
