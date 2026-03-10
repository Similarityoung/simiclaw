package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/ingest"
	"github.com/similarityyoung/simiclaw/internal/session"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/internal/streaming"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type captureEnqueuer struct {
	eventIDs []string
}

func (q *captureEnqueuer) TryEnqueue(eventID string) bool {
	q.eventIDs = append(q.eventIDs, eventID)
	return true
}

type captureSender struct {
	messages []model.OutboxMessage
}

func (s *captureSender) Send(_ context.Context, msg model.OutboxMessage) error {
	s.messages = append(s.messages, msg)
	return nil
}

func TestRunScheduledKindUsesUnifiedIngestSemantics(t *testing.T) {
	workspace := t.TempDir()
	if err := store.InitWorkspace(workspace, false, store.DefaultBusyTimeout()); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}
	db, err := store.Open(workspace, store.DefaultBusyTimeout())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC)
	conv := model.Conversation{ConversationID: "cron-conv", ChannelType: "dm", ParticipantID: "u1"}
	if _, err := db.Writer().ExecContext(
		context.Background(),
		`INSERT INTO conversation_scopes (tenant_id, conversation_id, channel_type, participant_id, dm_scope, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"local",
		conv.ConversationID,
		conv.ChannelType,
		conv.ParticipantID,
		"scope_saved",
		now.Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("insert conversation scope: %v", err)
	}

	payloadJSON, err := json.Marshal(store.ScheduledJobPayload{
		Source:   "cron",
		TenantID: "local",
		Conversation: model.Conversation{
			ConversationID: conv.ConversationID,
			ChannelType:    conv.ChannelType,
			ParticipantID:  conv.ParticipantID,
		},
		Payload: model.EventPayload{Type: "cron_fire", Text: "nightly heartbeat"},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if _, err := db.Writer().ExecContext(
		context.Background(),
		`INSERT INTO scheduled_jobs (
			job_id, name, kind, status, payload_json, next_run_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"cron:nightly",
		"nightly",
		string(model.ScheduledJobKindCron),
		string(model.ScheduledJobStatusActive),
		string(payloadJSON),
		now.Add(-time.Minute).Format(time.RFC3339Nano),
		now.Add(-time.Minute).Format(time.RFC3339Nano),
		now.Add(-time.Minute).Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("insert scheduled job: %v", err)
	}

	queue := &captureEnqueuer{}
	ingestService := ingest.NewService("local", db, queue, ingest.NewScopeResolver("local", db), 100, 100, 100, 100)
	supervisor := &Supervisor{
		db:     db,
		ingest: ingestService,
		ctx:    context.Background(),
	}

	supervisor.runScheduledKind(now, model.ScheduledJobKindCron)

	if len(queue.eventIDs) != 1 {
		t.Fatalf("expected one enqueued event, got %+v", queue.eventIDs)
	}
	event, ok, err := db.GetEvent(context.Background(), queue.eventIDs[0])
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if !ok {
		t.Fatalf("scheduled event not found")
	}
	wantSessionKey, err := session.ComputeKey("local", conv, "scope_saved")
	if err != nil {
		t.Fatalf("ComputeKey: %v", err)
	}
	if event.SessionKey != wantSessionKey {
		t.Fatalf("expected inherited session key %q, got %q", wantSessionKey, event.SessionKey)
	}
	if !strings.HasPrefix(event.PayloadHash, "sha256:") {
		t.Fatalf("expected canonical payload hash from unified ingest service, got %q", event.PayloadHash)
	}
	if event.Status != model.EventStatusQueued {
		t.Fatalf("expected queued event, got %+v", event)
	}
}

func TestHubStreamSinkPublishesEventsAndTerminalHelpers(t *testing.T) {
	hub := streaming.NewHub()
	sub := hub.Reserve("idem")
	defer hub.Release(sub)
	if terminal := hub.Attach(sub, "evt_stream"); terminal != nil {
		t.Fatalf("unexpected terminal event: %+v", terminal)
	}

	sink := newHubStreamSink(hub, "evt_stream")
	sink.OnStatus("processing", "claimed")
	sink.OnReasoningDelta("thinking")
	sink.OnTextDelta("hello")
	sink.OnToolStart("call_1", "memory_search", map[string]any{"query": "x"}, false)
	sink.OnToolResult("call_1", "memory_search", map[string]any{"hits": 1}, false, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, want := range []model.ChatStreamEventType{
		model.ChatStreamEventStatus,
		model.ChatStreamEventReasoningDelta,
		model.ChatStreamEventTextDelta,
		model.ChatStreamEventToolStart,
		model.ChatStreamEventToolResult,
	} {
		event, ok := sub.Next(ctx)
		if !ok {
			t.Fatalf("expected stream event %s", want)
		}
		if event.Type != want {
			t.Fatalf("expected event type %s, got %+v", want, event)
		}
	}

	failed := terminalEventFromFinalize(store.RunFinalize{
		EventID:     "evt_fail",
		RunID:       "run_fail",
		RunMode:     model.RunModeNormal,
		EventStatus: model.EventStatusFailed,
		SessionKey:  "local:dm:u1",
		SessionID:   "ses_1",
		Error:       &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: "boom"},
		Now:         time.Now().UTC(),
	})
	if failed.Type != model.ChatStreamEventError || failed.Error == nil {
		t.Fatalf("expected failed terminal event, got %+v", failed)
	}
	done := terminalEventFromRecord(model.EventRecord{
		EventID:   "evt_done",
		Status:    model.EventStatusProcessed,
		UpdatedAt: time.Now().UTC(),
	})
	if done.Type != model.ChatStreamEventDone {
		t.Fatalf("expected done terminal event, got %+v", done)
	}
	if nonZeroTime(time.Time{}).IsZero() {
		t.Fatalf("expected nonZeroTime to synthesize timestamp")
	}
}

func TestSupervisorStartStopAndReadyState(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	if err := store.InitWorkspace(workspace, false, cfg.DBBusyTimeout.Duration); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}
	db, err := store.Open(workspace, cfg.DBBusyTimeout.Duration)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	result, err := db.IngestEvent(context.Background(), cfg.TenantID, "local:dm:u1", model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "stale", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:stale:1",
		Timestamp:      now.Add(-5 * time.Minute).Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "stale"},
	}, "sha256:stale", now.Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("IngestEvent stale: %v", err)
	}
	if err := db.MarkEventQueued(context.Background(), result.EventID, now.Add(-5*time.Minute)); err != nil {
		t.Fatalf("MarkEventQueued stale: %v", err)
	}
	if _, ok, err := db.ClaimEvent(context.Background(), result.EventID, "run_stale", now.Add(-5*time.Minute)); err != nil || !ok {
		t.Fatalf("ClaimEvent stale ok=%v err=%v", ok, err)
	}
	if _, err := db.Writer().ExecContext(
		context.Background(),
		`INSERT INTO events (
			event_id, source, tenant_id, conversation_id, channel_type, participant_id,
			session_key, session_id, idempotency_key, payload_type, payload_text,
			payload_json, payload_hash, status, outbox_id, outbox_status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"evt_outbox",
		"cli",
		cfg.TenantID,
		"conv_outbox",
		"dm",
		"u1",
		"local:dm:u1",
		"ses_outbox",
		"cli:outbox:1",
		"message",
		"hello",
		`{"type":"message","text":"hello"}`,
		"sha256:outbox",
		string(model.EventStatusProcessed),
		"out_1",
		string(model.OutboxStatusPending),
		now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("insert processed event: %v", err)
	}
	if _, err := db.Writer().ExecContext(
		context.Background(),
		`INSERT INTO outbox (
			outbox_id, event_id, session_key, channel, target_id, body, status, next_attempt_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"out_1",
		"evt_outbox",
		"local:dm:u1",
		"",
		"",
		"hello",
		string(model.OutboxStatusPending),
		now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}

	loop := NewEventLoop(db, fixedOutputRunner{}, streaming.NewHub(), 8, 1)
	ingestService := ingest.NewService(cfg.TenantID, db, loop, ingest.NewScopeResolver(cfg.TenantID, db), 100, 100, 100, 100)
	sender := &captureSender{}
	supervisor := NewSupervisor(cfg, db, ingestService, loop, sender)
	supervisor.Start()
	defer supervisor.Stop()

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok, _ := db.HeartbeatAt(context.Background(), "heartbeat"); ok && len(sender.messages) == 1 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !supervisor.EventLoopAlive() || !loop.IsAlive() || loop.InboundDepth() != 0 {
		t.Fatalf("expected running event loop")
	}
	state, err := supervisor.ReadyState(context.Background())
	if err != nil {
		t.Fatalf("ReadyState: %v state=%+v", err, state)
	}
	for _, worker := range []string{"heartbeat", "processing_sweeper", "outbox_retry", "delayed_jobs", "cron"} {
		if state[worker] != "alive" {
			t.Fatalf("expected worker %s alive, got %+v", worker, state)
		}
	}
	staleEvent, ok, err := db.GetEvent(context.Background(), result.EventID)
	if err != nil || !ok || staleEvent.Status == model.EventStatusProcessing {
		t.Fatalf("expected stale event to be recovered, ok=%v err=%v event=%+v", ok, err, staleEvent)
	}
	sentEvent, ok, err := db.GetEvent(context.Background(), "evt_outbox")
	if err != nil || !ok || sentEvent.OutboxStatus != model.OutboxStatusSent {
		t.Fatalf("expected outbox worker to send message, ok=%v err=%v event=%+v sender=%+v", ok, err, sentEvent, sender.messages)
	}
}

func TestTelegramTargetIDValidation(t *testing.T) {
	if _, err := telegramTargetID(model.InternalEvent{Payload: model.EventPayload{Extra: map[string]string{}}}); err == nil {
		t.Fatalf("expected missing telegram chat id error")
	}
	if _, err := telegramTargetID(model.InternalEvent{Payload: model.EventPayload{Extra: map[string]string{"telegram_chat_id": "bad"}}}); err == nil {
		t.Fatalf("expected invalid telegram chat id error")
	}
	got, err := telegramTargetID(model.InternalEvent{Payload: model.EventPayload{Extra: map[string]string{"telegram_chat_id": "42"}}})
	if err != nil || got != "42" {
		t.Fatalf("expected chat id 42, got %q err=%v", got, err)
	}
}
