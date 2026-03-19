package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/gateway"
	gatewaybindings "github.com/similarityyoung/simiclaw/internal/gateway/bindings"
	"github.com/similarityyoung/simiclaw/internal/runner"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/internal/store"
	storequeries "github.com/similarityyoung/simiclaw/internal/store/queries"
	storetx "github.com/similarityyoung/simiclaw/internal/store/tx"
	"github.com/similarityyoung/simiclaw/internal/streaming"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestEventLoopRecoversRunnerPanicAndPublishesTerminalError(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	if err := store.InitWorkspace(workspace, false, cfg.DBBusyTimeout.Duration); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}
	db, err := store.Open(workspace, cfg.DBBusyTimeout.Duration)
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}
	defer db.Close()
	repo := storetx.NewRuntimeRepository(db)
	queryRepo := storequeries.NewRepository(db)

	conversation := model.Conversation{ConversationID: "panic", ChannelType: "dm", ParticipantID: "u1"}
	sessionKey, err := gatewaybindings.ComputeKey(cfg.TenantID, conversation, "default")
	if err != nil {
		t.Fatalf("ComputeKey: %v", err)
	}
	req := api.IngestRequest{
		Source:         "cli",
		Conversation:   conversation,
		IdempotencyKey: "cli:panic:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}
	result, err := repo.PersistEvent(context.Background(), cfg.TenantID, sessionKey, gateway.PersistRequest{
		Source:         req.Source,
		Conversation:   req.Conversation,
		Payload:        req.Payload,
		IdempotencyKey: req.IdempotencyKey,
		DMScope:        req.DMScope,
	}, "payload-hash", time.Now().UTC())
	if err != nil {
		t.Fatalf("PersistEvent: %v", err)
	}
	if err := repo.MarkEventQueued(context.Background(), result.EventID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkEventQueued: %v", err)
	}

	hub := streaming.NewHub()
	sub := hub.Reserve(req.IdempotencyKey)
	defer hub.Release(sub)
	if replay := hub.Attach(sub, result.EventID); len(replay) > 0 {
		t.Fatalf("unexpected replay before processing: %+v", replay)
	}

	loop := NewEventLoop(repo, NewRunnerExecutor(panicRunner{}, 1), hub, 8)
	if err := loop.Start(context.Background()); err != nil {
		t.Fatalf("Start loop: %v", err)
	}
	defer loop.Stop()
	if !loop.TryEnqueue(result.EventID) {
		t.Fatalf("failed to enqueue event")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var terminal runtimemodel.RuntimeEvent
	for {
		event, ok := sub.Next(ctx)
		if !ok {
			t.Fatalf("stream closed before terminal event")
		}
		if event.IsTerminal() {
			terminal = event
			break
		}
	}
	if terminal.Kind != runtimemodel.RuntimeEventFailed {
		t.Fatalf("expected terminal error, got %+v", terminal)
	}

	eventRecord, ok, err := queryRepo.GetEventRecord(context.Background(), result.EventID)
	if err != nil {
		t.Fatalf("GetEventRecord: %v", err)
	}
	if !ok {
		t.Fatalf("event not found")
	}
	if eventRecord.Status != model.EventStatusFailed {
		t.Fatalf("expected failed event, got %+v", eventRecord)
	}
}

type panicRunner struct{}

func (panicRunner) Run(context.Context, model.InternalEvent, int, runner.StreamSink) (runner.RunOutput, error) {
	panic("boom")
}

type fixedOutputRunner struct {
	output runner.RunOutput
	err    error
}

func (r fixedOutputRunner) Run(context.Context, model.InternalEvent, int, runner.StreamSink) (runner.RunOutput, error) {
	return r.output, r.err
}

func TestEventLoopFailsTelegramReplyWithoutChatID(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	if err := store.InitWorkspace(workspace, false, cfg.DBBusyTimeout.Duration); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}
	db, err := store.Open(workspace, cfg.DBBusyTimeout.Duration)
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}
	defer db.Close()
	repo := storetx.NewRuntimeRepository(db)
	queryRepo := storequeries.NewRepository(db)

	conversation := model.Conversation{ConversationID: "tg_chat_42", ChannelType: "dm", ParticipantID: "1001"}
	sessionKey, err := gatewaybindings.ComputeKey(cfg.TenantID, conversation, "default")
	if err != nil {
		t.Fatalf("ComputeKey: %v", err)
	}
	now := time.Now().UTC()
	result, err := repo.PersistEvent(context.Background(), cfg.TenantID, sessionKey, gateway.PersistRequest{
		Source:         "telegram",
		Conversation:   conversation,
		IdempotencyKey: "telegram:update:1",
		Payload: model.EventPayload{
			Type:  "message",
			Text:  "hello",
			Extra: map[string]string{},
		},
	}, "payload-hash", now)
	if err != nil {
		t.Fatalf("PersistEvent: %v", err)
	}
	if err := repo.MarkEventQueued(context.Background(), result.EventID, now); err != nil {
		t.Fatalf("MarkEventQueued: %v", err)
	}

	hub := streaming.NewHub()
	loop := NewEventLoop(repo, NewRunnerExecutor(fixedOutputRunner{output: runner.RunOutput{AssistantReply: "reply", RunMode: model.RunModeNormal}}, 1), hub, 8)
	if err := loop.Start(context.Background()); err != nil {
		t.Fatalf("Start loop: %v", err)
	}
	defer loop.Stop()
	if !loop.TryEnqueue(result.EventID) {
		t.Fatal("failed to enqueue event")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec, ok, err := queryRepo.GetEventRecord(context.Background(), result.EventID)
		if err != nil {
			t.Fatalf("GetEventRecord: %v", err)
		}
		if !ok {
			t.Fatal("event not found")
		}
		if rec.Status == model.EventStatusFailed {
			if rec.OutboxStatus != "" {
				t.Fatalf("expected no outbox status, got %+v", rec)
			}
			if rec.AssistantReply != "" {
				t.Fatalf("expected assistant reply to be cleared, got %+v", rec)
			}
			if rec.Error == nil || rec.Error.Message == "" {
				t.Fatalf("expected terminal error, got %+v", rec)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timeout waiting for failed telegram event")
}

func TestEventLoopStopCancelsInFlightRun(t *testing.T) {
	repo := &stopPathFacts{}
	run := &stopAwareRunner{
		started:  make(chan struct{}),
		finished: make(chan struct{}),
	}
	hub := streaming.NewHub()
	loop := NewEventLoop(repo, NewRunnerExecutor(run, 1), hub, 1)
	if err := loop.Start(context.Background()); err != nil {
		t.Fatalf("Start loop: %v", err)
	}

	if !loop.TryEnqueue("evt_stop") {
		t.Fatalf("failed to enqueue event")
	}

	select {
	case <-run.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for in-flight run to start")
	}

	stopDone := make(chan struct{})
	go func() {
		loop.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event loop stop")
	}

	select {
	case <-run.finished:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runner cancellation")
	}

	if loop.IsAlive() {
		t.Fatalf("expected event loop to be down after stop")
	}
	if got := len(repo.finalizeCmds); got != 1 {
		t.Fatalf("expected one finalize command, got %d", got)
	}
	finalize := repo.finalizeCmds[0]
	if finalize.RunStatus != model.RunStatusFailed || finalize.EventStatus != model.EventStatusFailed {
		t.Fatalf("expected canceled in-flight run to finalize as failed, got %+v", finalize)
	}
	if finalize.Error == nil || finalize.Error.Message == "" {
		t.Fatalf("expected cancellation error in finalize command, got %+v", finalize)
	}
}

func TestEventLoopHydratesSessionLaneBeforeClaim(t *testing.T) {
	repo := &laneHydrationFacts{}
	loop := NewEventLoop(repo, NewRunnerExecutor(fixedOutputRunner{output: runner.RunOutput{RunMode: model.RunModeNormal}}, 1), nil, 1)
	if err := loop.Start(context.Background()); err != nil {
		t.Fatalf("Start loop: %v", err)
	}
	defer loop.Stop()

	loop.processWork(runtimemodel.WorkItem{
		Kind:     runtimemodel.WorkKindEvent,
		EventID:  "evt_lane",
		Identity: "evt_lane",
	})

	if repo.claimedWork.SessionKey != "local:dm:u1" {
		t.Fatalf("expected session key to be hydrated before claim, got %+v", repo.claimedWork)
	}
	if repo.claimedWork.LaneKey != "session:local:dm:u1" {
		t.Fatalf("expected session lane before claim, got %+v", repo.claimedWork)
	}
}

type stopAwareRunner struct {
	once     sync.Once
	started  chan struct{}
	finished chan struct{}
}

func (r *stopAwareRunner) Run(ctx context.Context, _ model.InternalEvent, _ int, _ runner.StreamSink) (runner.RunOutput, error) {
	r.once.Do(func() {
		close(r.started)
	})
	<-ctx.Done()
	close(r.finished)
	return runner.RunOutput{RunMode: model.RunModeNormal}, ctx.Err()
}

type stopPathFacts struct {
	mu           sync.Mutex
	finalizeCmds []runtimemodel.RunFinalize
}

func (r *stopPathFacts) ListRunnable(context.Context, int) ([]runtimemodel.WorkItem, error) {
	return nil, nil
}

func (r *stopPathFacts) ClaimWork(_ context.Context, work runtimemodel.WorkItem, runID string, _ time.Time) (runtimemodel.ClaimContext, bool, error) {
	eventID := work.EventID
	if eventID == "" {
		eventID = work.Identity
	}
	return runtimemodel.ClaimContext{
		Work: work,
		Event: model.InternalEvent{
			EventID:         eventID,
			Source:          "cli",
			TenantID:        "local",
			SessionKey:      "local:dm:u1",
			ActiveSessionID: "ses_stop",
			Payload:         model.EventPayload{Type: "message", Text: "stop"},
		},
		RunID:      runID,
		RunMode:    model.RunModeNormal,
		SessionKey: "local:dm:u1",
		SessionID:  "ses_stop",
		Source:     "cli",
	}, true, nil
}

func (r *stopPathFacts) Finalize(_ context.Context, finalize runtimemodel.FinalizeCommand) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeCmds = append(r.finalizeCmds, finalize)
	return nil
}

func (r *stopPathFacts) GetEventRecord(context.Context, string) (runtimemodel.EventRecord, bool, error) {
	return runtimemodel.EventRecord{}, false, nil
}

type laneHydrationFacts struct {
	claimedWork runtimemodel.WorkItem
}

func (r *laneHydrationFacts) ListRunnable(context.Context, int) ([]runtimemodel.WorkItem, error) {
	return nil, nil
}

func (r *laneHydrationFacts) ClaimWork(_ context.Context, work runtimemodel.WorkItem, runID string, _ time.Time) (runtimemodel.ClaimContext, bool, error) {
	r.claimedWork = work
	return runtimemodel.ClaimContext{
		Work: work,
		Event: model.InternalEvent{
			EventID:         work.EventID,
			Source:          "cli",
			TenantID:        "local",
			SessionKey:      work.SessionKey,
			ActiveSessionID: "ses_lane",
			Payload:         model.EventPayload{Type: "message", Text: "lane"},
		},
		RunID:      runID,
		RunMode:    model.RunModeNormal,
		SessionKey: work.SessionKey,
		SessionID:  "ses_lane",
		Source:     "cli",
	}, true, nil
}

func (r *laneHydrationFacts) Finalize(context.Context, runtimemodel.FinalizeCommand) error {
	return nil
}

func (r *laneHydrationFacts) GetEventRecord(context.Context, string) (runtimemodel.EventRecord, bool, error) {
	return runtimemodel.EventRecord{
		EventID:    "evt_lane",
		SessionKey: "local:dm:u1",
	}, true, nil
}
