package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/ingest/port"
	"github.com/similarityyoung/simiclaw/internal/runner"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/internal/session"
	"github.com/similarityyoung/simiclaw/internal/store"
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

	conversation := model.Conversation{ConversationID: "panic", ChannelType: "dm", ParticipantID: "u1"}
	sessionKey, err := session.ComputeKey(cfg.TenantID, conversation, "default")
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
	result, err := db.IngestEvent(context.Background(), cfg.TenantID, sessionKey, port.PersistRequest{
		Source:         req.Source,
		Conversation:   req.Conversation,
		Payload:        req.Payload,
		IdempotencyKey: req.IdempotencyKey,
		DMScope:        req.DMScope,
	}, "payload-hash", time.Now().UTC())
	if err != nil {
		t.Fatalf("IngestEvent: %v", err)
	}
	if err := db.MarkEventQueued(context.Background(), result.EventID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkEventQueued: %v", err)
	}

	hub := streaming.NewHub()
	sub := hub.Reserve(req.IdempotencyKey)
	defer hub.Release(sub)
	if terminal := hub.Attach(sub, result.EventID); terminal != nil {
		t.Fatalf("unexpected terminal before processing: %+v", terminal)
	}

	loop := NewEventLoop(db, panicRunner{}, hub, 8, 1)
	loop.Start()
	defer loop.Stop()
	if !loop.TryEnqueue(result.EventID) {
		t.Fatalf("failed to enqueue event")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var terminal api.ChatStreamEvent
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
	if terminal.Type != api.ChatStreamEventError {
		t.Fatalf("expected terminal error, got %+v", terminal)
	}

	eventRecord, ok, err := db.GetEvent(context.Background(), result.EventID)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
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

	conversation := model.Conversation{ConversationID: "tg_chat_42", ChannelType: "dm", ParticipantID: "1001"}
	sessionKey, err := session.ComputeKey(cfg.TenantID, conversation, "default")
	if err != nil {
		t.Fatalf("ComputeKey: %v", err)
	}
	now := time.Now().UTC()
	result, err := db.IngestEvent(context.Background(), cfg.TenantID, sessionKey, port.PersistRequest{
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
		t.Fatalf("IngestEvent: %v", err)
	}
	if err := db.MarkEventQueued(context.Background(), result.EventID, now); err != nil {
		t.Fatalf("MarkEventQueued: %v", err)
	}

	loop := NewEventLoop(db, fixedOutputRunner{output: runner.RunOutput{AssistantReply: "reply", RunMode: model.RunModeNormal}}, streaming.NewHub(), 8, 1)
	loop.Start()
	defer loop.Stop()
	if !loop.TryEnqueue(result.EventID) {
		t.Fatal("failed to enqueue event")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec, ok, err := db.GetEvent(context.Background(), result.EventID)
		if err != nil {
			t.Fatalf("GetEvent: %v", err)
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
	repo := &stopPathRepo{}
	run := &stopAwareRunner{
		started:  make(chan struct{}),
		finished: make(chan struct{}),
	}
	loop := NewEventLoop(repo, run, streaming.NewHub(), 1, 1)
	loop.Start()

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

type stopPathRepo struct {
	mu           sync.Mutex
	finalizeCmds []runtimemodel.RunFinalize
}

func (r *stopPathRepo) ListRunnableEventIDs(context.Context, int) ([]string, error) {
	return nil, nil
}

func (r *stopPathRepo) ClaimLoopEvent(_ context.Context, eventID, runID string, _ time.Time) (runtimemodel.ClaimedEvent, bool, error) {
	return runtimemodel.ClaimedEvent{
		Event: model.InternalEvent{
			EventID:         eventID,
			Source:          "cli",
			TenantID:        "local",
			SessionKey:      "local:dm:u1",
			ActiveSessionID: "ses_stop",
			Payload:         model.EventPayload{Type: "message", Text: "stop"},
		},
		RunID:   runID,
		Status:  model.EventStatusProcessing,
		RunMode: model.RunModeNormal,
	}, true, nil
}

func (r *stopPathRepo) FinalizeLoopRun(_ context.Context, finalize runtimemodel.RunFinalize) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeCmds = append(r.finalizeCmds, finalize)
	return nil
}

func (r *stopPathRepo) GetLoopEventRecord(context.Context, string) (runtimemodel.EventRecord, bool, error) {
	return runtimemodel.EventRecord{}, false, nil
}
