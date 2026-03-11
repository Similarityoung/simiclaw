package store

import (
	"context"
	"encoding/json"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestLookupInboundListCollectionsAndCheckReadWrite(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Now().UTC()

	result, err := db.IngestEvent(ctx, "local", "local:dm:u1", persistRequest(api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "list", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:list:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello list"},
	}), "sha256:list", now)
	if err != nil {
		t.Fatalf("IngestEvent: %v", err)
	}
	lookup, ok, err := db.LookupInbound(ctx, "cli:list:1")
	if err != nil || !ok || lookup.EventID != result.EventID {
		t.Fatalf("LookupInbound ok=%v err=%v lookup=%+v", ok, err, lookup)
	}
	if err := db.MarkEventQueued(ctx, result.EventID, now); err != nil {
		t.Fatalf("MarkEventQueued: %v", err)
	}
	runnable, err := db.ListRunnableEventIDs(ctx, 10)
	if err != nil || len(runnable) != 1 || runnable[0] != result.EventID {
		t.Fatalf("ListRunnableEventIDs err=%v ids=%+v", err, runnable)
	}
	claimed, ok, err := db.ClaimEvent(ctx, result.EventID, "run_list_1", now)
	if err != nil || !ok {
		t.Fatalf("ClaimEvent ok=%v err=%v", ok, err)
	}
	if err := db.FinalizeRun(ctx, RunFinalize{
		RunID:             claimed.RunID,
		EventID:           claimed.Event.EventID,
		SessionKey:        claimed.Event.SessionKey,
		SessionID:         claimed.Event.ActiveSessionID,
		RunMode:           model.RunModeNormal,
		RunStatus:         model.RunStatusCompleted,
		EventStatus:       model.EventStatusProcessed,
		AssistantReply:    "done",
		OutboxBody:        "done",
		Provider:          "fake",
		Model:             "default",
		ProviderRequestID: "req_1",
		Messages: []StoredMessage{
			{MessageID: "msg_list_user", SessionKey: claimed.Event.SessionKey, SessionID: claimed.Event.ActiveSessionID, RunID: claimed.RunID, Role: "user", Content: "hello list", Visible: true, CreatedAt: now},
			{MessageID: "msg_list_assistant", SessionKey: claimed.Event.SessionKey, SessionID: claimed.Event.ActiveSessionID, RunID: claimed.RunID, Role: "assistant", Content: "done", Visible: true, CreatedAt: now},
		},
		Now: now,
	}); err != nil {
		t.Fatalf("FinalizeRun: %v", err)
	}
	if err := db.CheckReadWrite(ctx); err != nil {
		t.Fatalf("CheckReadWrite: %v", err)
	}

	events, err := db.ListEventsPage(ctx, EventListFilter{Limit: 10})
	if err != nil || len(events) == 0 {
		t.Fatalf("ListEventsPage err=%v events=%+v", err, events)
	}
	runs, err := db.ListRunsPage(ctx, RunListFilter{Limit: 10})
	if err != nil || len(runs) == 0 {
		t.Fatalf("ListRunsPage err=%v runs=%+v", err, runs)
	}
	sessions, err := db.ListSessionsPage(ctx, SessionListFilter{Limit: 10})
	if err != nil || len(sessions) == 0 {
		t.Fatalf("ListSessionsPage err=%v sessions=%+v", err, sessions)
	}
}

func TestListMessagesAndOutboxTransitions(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Now().UTC()

	result, err := db.IngestEvent(ctx, "local", "local:dm:u1", persistRequest(api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "messages", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:messages:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello messages"},
	}), "sha256:messages", now)
	if err != nil {
		t.Fatalf("IngestEvent: %v", err)
	}
	if err := db.MarkEventQueued(ctx, result.EventID, now); err != nil {
		t.Fatalf("MarkEventQueued: %v", err)
	}
	claimed, ok, err := db.ClaimEvent(ctx, result.EventID, "run_messages_1", now)
	if err != nil || !ok {
		t.Fatalf("ClaimEvent ok=%v err=%v", ok, err)
	}
	if err := db.FinalizeRun(ctx, RunFinalize{
		RunID:          claimed.RunID,
		EventID:        claimed.Event.EventID,
		SessionKey:     claimed.Event.SessionKey,
		SessionID:      claimed.Event.ActiveSessionID,
		RunMode:        model.RunModeNormal,
		RunStatus:      model.RunStatusCompleted,
		EventStatus:    model.EventStatusProcessed,
		AssistantReply: "done",
		OutboxBody:     "done",
		Messages: []StoredMessage{
			{MessageID: "msg_hidden_assistant", SessionKey: claimed.Event.SessionKey, SessionID: claimed.Event.ActiveSessionID, RunID: claimed.RunID, Role: "assistant", Content: "thinking", Visible: false, CreatedAt: now},
			{MessageID: "msg_visible_user", SessionKey: claimed.Event.SessionKey, SessionID: claimed.Event.ActiveSessionID, RunID: claimed.RunID, Role: "user", Content: "hello messages", Visible: true, CreatedAt: now},
			{MessageID: "msg_visible_assistant", SessionKey: claimed.Event.SessionKey, SessionID: claimed.Event.ActiveSessionID, RunID: claimed.RunID, Role: "assistant", Content: "done", Visible: true, CreatedAt: now},
		},
		Now: now,
	}); err != nil {
		t.Fatalf("FinalizeRun: %v", err)
	}

	visible, err := db.ListMessages(ctx, claimed.Event.ActiveSessionID, 10, time.Time{}, "", true)
	if err != nil || len(visible) != 2 {
		t.Fatalf("ListMessages visible err=%v items=%+v", err, visible)
	}
	all, err := db.ListMessages(ctx, claimed.Event.ActiveSessionID, 10, time.Time{}, "", false)
	if err != nil || len(all) != 3 {
		t.Fatalf("ListMessages all err=%v items=%+v", err, all)
	}

	outbox, ok, err := db.ClaimOutbox(ctx, "worker", now)
	if err != nil || !ok {
		t.Fatalf("ClaimOutbox ok=%v err=%v", ok, err)
	}
	if err := db.FailOutboxSend(ctx, outbox.OutboxID, outbox.EventID, "retry", false, now, now); err != nil {
		t.Fatalf("FailOutboxSend: %v", err)
	}
	reclaimed, ok, err := db.ClaimOutbox(ctx, "worker", now)
	if err != nil || !ok || reclaimed.OutboxID != outbox.OutboxID {
		t.Fatalf("re-ClaimOutbox ok=%v err=%v reclaimed=%+v", ok, err, reclaimed)
	}
	if err := db.CompleteOutboxSend(ctx, reclaimed.OutboxID, reclaimed.EventID, now); err != nil {
		t.Fatalf("CompleteOutboxSend: %v", err)
	}
	event, ok, err := db.GetEvent(ctx, reclaimed.EventID)
	if err != nil || !ok || event.OutboxStatus != model.OutboxStatusSent {
		t.Fatalf("GetEvent ok=%v err=%v event=%+v", ok, err, event)
	}
}

func TestCronJobsHeartbeatsAndHelpers(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Now().UTC()

	if err := db.UpsertCronJobs(ctx, "local", []config.CronJobConfig{{
		Name:           "nightly",
		ConversationID: "conv-nightly",
		ChannelType:    "dm",
		ParticipantID:  "u1",
		PayloadType:    "cron_fire",
		PayloadText:    "nightly heartbeat",
		Interval:       config.Duration{Duration: time.Second},
	}}, now); err != nil {
		t.Fatalf("UpsertCronJobs: %v", err)
	}
	claimed, ok, err := db.ClaimScheduledJob(ctx, model.ScheduledJobKindCron, "cron-worker", now.Add(2*time.Second))
	if err != nil || !ok {
		t.Fatalf("ClaimScheduledJob ok=%v err=%v", ok, err)
	}
	if err := db.CompleteScheduledJob(ctx, claimed, now.Add(2*time.Second)); err != nil {
		t.Fatalf("CompleteScheduledJob cron: %v", err)
	}
	delayed := ClaimedJob{JobID: "delayed:1", Kind: model.ScheduledJobKindDelayed, Status: model.ScheduledJobStatusActive}
	if _, err := db.Writer().ExecContext(
		ctx,
		`INSERT INTO scheduled_jobs (
			job_id, name, kind, status, payload_json, next_run_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, '{}', ?, ?, ?)`,
		delayed.JobID,
		"delayed",
		string(delayed.Kind),
		string(delayed.Status),
		timeText(now),
		timeText(now),
		timeText(now),
	); err != nil {
		t.Fatalf("insert delayed job: %v", err)
	}
	if err := db.CompleteScheduledJob(ctx, delayed, now); err != nil {
		t.Fatalf("CompleteScheduledJob delayed: %v", err)
	}
	if err := db.FailScheduledJob(ctx, delayed.JobID, "retry later", now.Add(time.Minute), now); err != nil {
		t.Fatalf("FailScheduledJob: %v", err)
	}
	if err := db.BeatHeartbeat(ctx, "worker-a", now); err != nil {
		t.Fatalf("BeatHeartbeat: %v", err)
	}
	beatAt, ok, err := db.HeartbeatAt(ctx, "worker-a")
	if err != nil || !ok || !beatAt.Equal(now) {
		t.Fatalf("HeartbeatAt ok=%v err=%v beatAt=%v", ok, err, beatAt)
	}
	if _, ok, err := db.HeartbeatAt(ctx, "missing"); err != nil || ok {
		t.Fatalf("expected missing heartbeat, ok=%v err=%v", ok, err)
	}
}

func TestFSAndJSONHelpers(t *testing.T) {
	jsonPath := filepath.Join(t.TempDir(), "config.json")
	body, err := json.MarshalIndent(map[string]string{"status": "ok"}, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent: %v", err)
	}
	body = append(body, '\n')
	if err := AtomicWriteFile(jsonPath, body, 0o644); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded["status"] != "ok" {
		t.Fatalf("unexpected decoded content: %+v", decoded)
	}
}

func TestSearchMessagesFTSAndGetters(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Now().UTC()
	result, err := db.IngestEvent(ctx, "local", "local:dm:u1", persistRequest(api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "fts", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:fts:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "alpha search"},
	}), "sha256:fts", now)
	if err != nil {
		t.Fatalf("IngestEvent: %v", err)
	}
	if err := db.MarkEventQueued(ctx, result.EventID, now); err != nil {
		t.Fatalf("MarkEventQueued: %v", err)
	}
	claimed, ok, err := db.ClaimEvent(ctx, result.EventID, "run_fts", now)
	if err != nil || !ok {
		t.Fatalf("ClaimEvent ok=%v err=%v", ok, err)
	}
	if err := db.FinalizeRun(ctx, RunFinalize{
		RunID:       claimed.RunID,
		EventID:     claimed.Event.EventID,
		SessionKey:  claimed.Event.SessionKey,
		SessionID:   claimed.Event.ActiveSessionID,
		RunMode:     model.RunModeNormal,
		RunStatus:   model.RunStatusCompleted,
		EventStatus: model.EventStatusProcessed,
		Messages: []StoredMessage{
			{MessageID: "msg_fts_user", SessionKey: claimed.Event.SessionKey, SessionID: claimed.Event.ActiveSessionID, RunID: claimed.RunID, Role: "user", Content: "alpha search", Visible: true, CreatedAt: now},
		},
		Now: now,
	}); err != nil {
		t.Fatalf("FinalizeRun: %v", err)
	}

	hits, err := db.SearchMessagesFTS(ctx, claimed.Event.ActiveSessionID, "alpha", 5)
	if err != nil || len(hits) != 1 {
		t.Fatalf("SearchMessagesFTS err=%v hits=%+v", err, hits)
	}
	empty, err := db.SearchMessagesFTS(ctx, claimed.Event.ActiveSessionID, "", 5)
	if err != nil || empty != nil {
		t.Fatalf("expected empty query to return nil, err=%v hits=%+v", err, empty)
	}
	run, ok, err := db.GetRun(ctx, claimed.RunID)
	if err != nil || !ok || run.RunID != claimed.RunID {
		t.Fatalf("GetRun ok=%v err=%v run=%+v", ok, err, run)
	}
	session, ok, err := db.GetSession(ctx, claimed.Event.SessionKey)
	if err != nil || !ok || session.SessionKey != claimed.Event.SessionKey {
		t.Fatalf("GetSession ok=%v err=%v session=%+v", ok, err, session)
	}
}
