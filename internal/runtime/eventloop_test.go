package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/runner"
	"github.com/similarityyoung/simiclaw/internal/session"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/internal/streaming"
	"github.com/similarityyoung/simiclaw/pkg/config"
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
	req := model.IngestRequest{
		Source:         "cli",
		Conversation:   conversation,
		IdempotencyKey: "cli:panic:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}
	result, err := db.IngestEvent(context.Background(), cfg.TenantID, sessionKey, req, "payload-hash", time.Now().UTC())
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
	var terminal model.ChatStreamEvent
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
	if terminal.Type != model.ChatStreamEventError {
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
