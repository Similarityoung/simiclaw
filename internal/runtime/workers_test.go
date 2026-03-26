package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/gateway"
	gatewaybindings "github.com/similarityyoung/simiclaw/internal/gateway/bindings"
	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
	gatewayrouting "github.com/similarityyoung/simiclaw/internal/gateway/routing"
	runtimeevents "github.com/similarityyoung/simiclaw/internal/runtime/events"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	runtimepayload "github.com/similarityyoung/simiclaw/internal/runtime/payload"
	runtimeworkers "github.com/similarityyoung/simiclaw/internal/runtime/workers"
	"github.com/similarityyoung/simiclaw/internal/store"
	storequeries "github.com/similarityyoung/simiclaw/internal/store/queries"
	storetx "github.com/similarityyoung/simiclaw/internal/store/tx"
	"github.com/similarityyoung/simiclaw/internal/testutil/logcapture"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type captureEnqueuer struct {
	eventIDs []string
}

func (q *captureEnqueuer) TryEnqueue(eventID string) bool {
	q.eventIDs = append(q.eventIDs, eventID)
	return true
}

type rejectEnqueuer struct{}

func (q *rejectEnqueuer) TryEnqueue(string) bool {
	return false
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
	queryRepo := storequeries.NewRepository(db)

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
	repo := storetx.NewRuntimeRepository(db)
	ingestService := newGatewayIngestor("local", repo, db, queryRepo, queue)
	ingestService.SetClock(func() time.Time { return now })

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		runtimeworkers.RunScheduledKind(context.Background(), repo, ingestService, nil, model.ScheduledJobKindCron, now)
		_ = logging.Sync()
	})

	if len(queue.eventIDs) != 1 {
		t.Fatalf("expected one enqueued event, got %+v", queue.eventIDs)
	}
	event, ok, err := queryRepo.GetEventRecord(context.Background(), queue.eventIDs[0])
	if err != nil {
		t.Fatalf("GetEventRecord: %v", err)
	}
	if !ok {
		t.Fatalf("scheduled event not found")
	}
	wantSessionKey, err := gatewaybindings.ComputeKey("local", conv, "scope_saved")
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
	if !strings.Contains(out, "[runtime.worker] job claimed") || !strings.Contains(out, `"job_id": "cron:nightly"`) {
		t.Fatalf("missing scheduled-job claim log in %q", out)
	}
	if !strings.Contains(out, "[runtime.worker] job enqueued") || !strings.Contains(out, `"event_id": "`+queue.eventIDs[0]+`"`) {
		t.Fatalf("missing scheduled-job enqueue log in %q", out)
	}
}

func TestRunScheduledKindFallbackLoopMarksEventQueued(t *testing.T) {
	db := newRuntimeTestDB(t)
	now := time.Date(2026, 3, 10, 16, 0, 0, 0, time.UTC)
	conv := model.Conversation{ConversationID: "cron-fallback", ChannelType: "dm", ParticipantID: "u2"}
	payloadJSON, err := json.Marshal(store.ScheduledJobPayload{
		Source:       "cron",
		TenantID:     "local",
		Conversation: conv,
		Payload:      model.EventPayload{Type: "cron_fire", Text: "fallback enqueue"},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if _, err := db.Writer().ExecContext(
		context.Background(),
		`INSERT INTO scheduled_jobs (
			job_id, name, kind, status, payload_json, next_run_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"cron:fallback",
		"fallback",
		string(model.ScheduledJobKindCron),
		string(model.ScheduledJobStatusActive),
		string(payloadJSON),
		now.Add(-time.Minute).Format(time.RFC3339Nano),
		now.Add(-time.Minute).Format(time.RFC3339Nano),
		now.Add(-time.Minute).Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("insert scheduled job: %v", err)
	}

	hub := runtimeevents.NewHub()
	repo := storetx.NewRuntimeRepository(db)
	queryRepo := storequeries.NewRepository(db)
	loop := NewEventLoop(repo, testQueryEventView{query: queryRepo}, NewRunnerExecutor(fixedOutputRunner{}, 1), hub, 4)
	ingestService := newGatewayIngestor("local", repo, db, queryRepo, &rejectEnqueuer{})
	ingestService.SetClock(func() time.Time { return now })

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		runtimeworkers.RunScheduledKind(context.Background(), repo, ingestService, loop, model.ScheduledJobKindCron, now)
		_ = logging.Sync()
	})

	if got := loop.InboundDepth(); got != 1 {
		t.Fatalf("expected one event queued into fallback loop, got depth=%d", got)
	}
	items, err := repo.ListRunnable(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRunnable: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one runnable event after fallback mark-queued, got %+v", items)
	}
	event, ok, err := queryRepo.GetEventRecord(context.Background(), items[0].EventID)
	if err != nil {
		t.Fatalf("GetEventRecord: %v", err)
	}
	if !ok {
		t.Fatalf("fallback event not found")
	}
	if items[0].SessionKey != event.SessionKey {
		t.Fatalf("expected runnable work to carry session key %q, got %+v", event.SessionKey, items[0])
	}
	if items[0].LaneKey != "session:"+event.SessionKey {
		t.Fatalf("expected runnable work to expose session lane, got %+v", items[0])
	}
	if event.Status != model.EventStatusQueued {
		t.Fatalf("expected fallback event to be queued, got %+v", event)
	}
	if !strings.Contains(out, "[runtime.worker] job fallback enqueued") {
		t.Fatalf("expected fallback enqueue log, got %q", out)
	}
}

func TestRuntimeEventStreamSinkPublishesEvents(t *testing.T) {
	hub := runtimeevents.NewHub()
	sub := hub.Reserve()
	defer hub.Release(sub)
	if replay := hub.Attach(sub, "evt_stream"); len(replay) > 0 {
		t.Fatalf("unexpected replay: %+v", replay)
	}

	sink := newRuntimeEventStreamSink(context.Background(), runtimemodel.ClaimContext{
		Work:       runtimemodel.WorkItem{EventID: "evt_stream"},
		Event:      model.InternalEvent{EventID: "evt_stream"},
		RunID:      "run_stream",
		SessionKey: "local:dm:u1",
		SessionID:  "ses_1",
	}, hub)
	sink.OnReasoningDelta("thinking")
	sink.OnTextDelta("hello")
	sink.OnToolStart("call_1", "memory_search", map[string]any{"query": "x"}, false)
	sink.OnToolResult("call_1", "memory_search", map[string]any{"hits": 1}, false, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, want := range []runtimemodel.RuntimeEventKind{
		runtimemodel.RuntimeEventReasoningDelta,
		runtimemodel.RuntimeEventTextDelta,
		runtimemodel.RuntimeEventToolStarted,
		runtimemodel.RuntimeEventToolFinished,
	} {
		event, ok := sub.Next(ctx)
		if !ok {
			t.Fatalf("expected stream event %s", want)
		}
		if event.Kind != want {
			t.Fatalf("expected event kind %s, got %+v", want, event)
		}
		if event.OccurredAt.IsZero() {
			t.Fatalf("expected stream event %s to carry a timestamp", want)
		}
	}
}

func TestHostControlStartStopAndReadinessProbe(t *testing.T) {
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
	repo := storetx.NewRuntimeRepository(db)
	queryRepo := storequeries.NewRepository(db)
	result, err := repo.PersistEvent(context.Background(), cfg.TenantID, "local:dm:u1", gateway.PersistRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "stale", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:stale:1",
		Payload:        model.EventPayload{Type: "message", Text: "stale"},
	}, "sha256:stale", now.Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("PersistEvent stale: %v", err)
	}
	if err := repo.MarkEventQueued(context.Background(), result.EventID, now.Add(-5*time.Minute)); err != nil {
		t.Fatalf("MarkEventQueued stale: %v", err)
	}
	if _, ok, err := repo.ClaimWork(context.Background(), runtimemodel.WorkItem{
		EventID: result.EventID,
	}, "run_stale", now.Add(-5*time.Minute)); err != nil || !ok {
		t.Fatalf("ClaimWork stale ok=%v err=%v", ok, err)
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

	hub := runtimeevents.NewHub()
	loop := NewEventLoop(repo, testQueryEventView{query: queryRepo}, NewRunnerExecutor(fixedOutputRunner{}, 1), hub, 8)
	ingestService := newGatewayIngestor(cfg.TenantID, repo, db, queryRepo, loop)
	ingestService.SetClock(func() time.Time { return now })
	sender := &captureSender{}
	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		host := NewHostControl(cfg, repo, ingestService, loop, sender)
		readiness := NewReadinessProbe(repo, host)
		if err := host.Start(context.Background()); err != nil {
			t.Fatalf("Start host control: %v", err)
		}

		deadline := time.Now().Add(12 * time.Second)
		for time.Now().Before(deadline) {
			if _, ok, _ := db.HeartbeatAt(context.Background(), "heartbeat"); ok && len(sender.messages) == 1 {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if !host.Alive() || !loop.IsAlive() || loop.InboundDepth() != 0 {
			t.Fatalf("expected running event loop")
		}
		state, err := readiness.ReadyState(context.Background())
		if err != nil {
			t.Fatalf("ReadyState: %v state=%+v", err, state)
		}
		for _, worker := range []string{"heartbeat", "processing_sweeper", "outbox_retry", "delayed_jobs", "cron"} {
			if state[worker] != "alive" {
				t.Fatalf("expected worker %s alive, got %+v", worker, state)
			}
		}
		staleEvent, ok, err := queryRepo.GetEventRecord(context.Background(), result.EventID)
		if err != nil || !ok || staleEvent.Status == model.EventStatusProcessing {
			t.Fatalf("expected stale event to be recovered, ok=%v err=%v event=%+v", ok, err, staleEvent)
		}
		sentEvent, ok, err := queryRepo.GetEventRecord(context.Background(), "evt_outbox")
		if err != nil || !ok || sentEvent.OutboxStatus != model.OutboxStatusSent {
			t.Fatalf("expected outbox worker to send message, ok=%v err=%v event=%+v sender=%+v", ok, err, sentEvent, sender.messages)
		}

		stopDone := make(chan struct{})
		go func() {
			host.Stop()
			close(stopDone)
		}()
		select {
		case <-stopDone:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for host control stop")
		}
		if host.Alive() || loop.IsAlive() {
			t.Fatalf("expected event loop to be down after host control stop")
		}
		state, err = readiness.ReadyState(context.Background())
		if err == nil || state["event_loop"] != "down" {
			t.Fatalf("expected loop-down readiness failure after stop, err=%v state=%+v", err, state)
		}
		_ = logging.Sync()
	})
	if !strings.Contains(out, "[runtime.worker] processing recovered") || !strings.Contains(out, `"count": 1`) {
		t.Fatalf("missing processing recovery summary in %q", out)
	}
	if !strings.Contains(out, "[outbound.delivery] send started") || !strings.Contains(out, `"outbox_id": "out_1"`) || !strings.Contains(out, "[outbound.delivery] sent") {
		t.Fatalf("missing delivery summary in %q", out)
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

func TestRuntimeEventStreamSinkHandlesNilHubAndEmptyDeltas(t *testing.T) {
	sink := newRuntimeEventStreamSink(context.Background(), runtimemodel.ClaimContext{
		Work:  runtimemodel.WorkItem{EventID: "evt_nil"},
		Event: model.InternalEvent{EventID: "evt_nil"},
	}, nil)
	sink.OnStatus("processing", "noop")
	sink.OnReasoningDelta("")
	sink.OnTextDelta("")
	sink.OnToolStart("call", "tool", nil, false)
	sink.OnToolResult("call", "tool", nil, false, nil)
}

func TestReadyStateWhenLoopDown(t *testing.T) {
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

	hub := runtimeevents.NewHub()
	repo := storetx.NewRuntimeRepository(db)
	loop := NewEventLoop(repo, nil, NewRunnerExecutor(fixedOutputRunner{}, 1), hub, 1)
	host := NewHostControl(cfg, repo, nil, loop, &captureSender{})
	readiness := NewReadinessProbe(repo, host)
	state, err := readiness.ReadyState(context.Background())
	if err == nil || state["event_loop"] != "down" {
		t.Fatalf("expected loop-down readiness failure, err=%v state=%+v", err, state)
	}
}

func TestRunScheduledKindFailsWithoutIngestService(t *testing.T) {
	db := newRuntimeTestDB(t)
	now := time.Now().UTC()
	if _, err := db.Writer().ExecContext(
		context.Background(),
		`INSERT INTO scheduled_jobs (
			job_id, name, kind, status, payload_json, next_run_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, '{}', ?, ?, ?)`,
		"cron:broken",
		"broken",
		string(model.ScheduledJobKindCron),
		string(model.ScheduledJobStatusActive),
		now.Add(-time.Minute).Format(time.RFC3339Nano),
		now.Add(-time.Minute).Format(time.RFC3339Nano),
		now.Add(-time.Minute).Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("insert scheduled job: %v", err)
	}

	repo := storetx.NewRuntimeRepository(db)
	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		runtimeworkers.RunScheduledKind(context.Background(), repo, nil, nil, model.ScheduledJobKindCron, now)
		_ = logging.Sync()
	})

	var lastError string
	if err := db.Reader().QueryRow(`SELECT last_error FROM scheduled_jobs WHERE job_id = 'cron:broken'`).Scan(&lastError); err != nil {
		t.Fatalf("read scheduled job: %v", err)
	}
	if lastError == "" {
		t.Fatalf("expected scheduled job failure to be recorded")
	}
	if !strings.Contains(out, "[runtime.worker] job ingest unavailable") || !strings.Contains(out, `"job_id": "cron:broken"`) {
		t.Fatalf("expected scheduled job failure summary, got %q", out)
	}
}

func TestNewEventLoopDefaultsAndQueueCapacity(t *testing.T) {
	loop := NewEventLoop(nil, nil, NewRunnerExecutor(fixedOutputRunner{}, 0), nil, 0)
	if cap(loop.queue) != 1024 {
		t.Fatalf("expected default queue capacity, got cap=%d", cap(loop.queue))
	}
	if !loop.TryEnqueue("evt_1") {
		t.Fatalf("expected first enqueue to succeed")
	}

	full := NewEventLoop(nil, nil, NewRunnerExecutor(fixedOutputRunner{}, 1), nil, 1)
	if !full.TryEnqueue("evt_1") {
		t.Fatalf("expected bounded queue first enqueue to succeed")
	}
	if full.TryEnqueue("evt_2") {
		t.Fatalf("expected bounded queue second enqueue to fail")
	}
}

func newRuntimeTestDB(t *testing.T) *store.DB {
	t.Helper()
	workspace := t.TempDir()
	if err := store.InitWorkspace(workspace, false, store.DefaultBusyTimeout()); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}
	db, err := store.Open(workspace, store.DefaultBusyTimeout())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

type gatewayEventIngestor struct {
	gateway *gateway.Service
}

func (g *gatewayEventIngestor) SetClock(now func() time.Time) {
	g.gateway.SetClock(now)
}

func (g *gatewayEventIngestor) Ingest(ctx context.Context, req runtimeworkers.IngestRequest) (runtimeworkers.IngestResult, error) {
	accepted, apiErr := g.gateway.Accept(ctx, gatewaymodel.NormalizedIngress{
		Source:         req.Source,
		Conversation:   req.Conversation,
		IdempotencyKey: req.IdempotencyKey,
		Timestamp:      req.Timestamp,
		Payload:        req.Payload,
	})
	if apiErr != nil {
		return runtimeworkers.IngestResult{}, apiErr
	}
	return runtimeworkers.IngestResult{
		EventID:   accepted.Result.EventID,
		Duplicate: accepted.Result.Duplicate,
		Enqueued:  accepted.Result.Enqueued,
	}, nil
}

func newGatewayIngestor(tenantID string, repo *storetx.RuntimeRepository, db *store.DB, queryRepo *storequeries.Repository, queue gateway.Enqueuer) *gatewayEventIngestor {
	payloads := runtimepayload.NewBuiltinRegistry()
	svc := gateway.NewService(
		tenantID,
		repo,
		queue,
		gatewaybindings.NewResolver(tenantID, newTestGatewaySessionLookup(db, queryRepo)),
		gatewayrouting.NewService(payloads),
		100,
		100,
		100,
		100,
	)
	return &gatewayEventIngestor{gateway: svc}
}
