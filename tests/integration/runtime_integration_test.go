//go:build integration

package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/bootstrap"
	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestIngestToProcessedAndQuerySQLite(t *testing.T) {
	app := newTestApp(t)

	req := api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "integration", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:integration:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello sqlite"},
	}
	resp := ingest(t, app, req, http.StatusAccepted)
	event := pollEvent(t, app, resp.EventID)
	if event.Status != model.EventStatusProcessed {
		t.Fatalf("expected processed, got %+v", event)
	}
	if event.OutboxStatus != model.OutboxStatusSent {
		t.Fatalf("expected sent outbox, got %+v", event)
	}
	if event.AssistantReply != "已收到: hello sqlite" {
		t.Fatalf("unexpected assistant reply: %+v", event)
	}

	runTrace := getRunTrace(t, app, event.RunID)
	if runTrace.Provider != "fake" || runTrace.Model != "default" {
		t.Fatalf("unexpected run trace provider/model: %+v", runTrace)
	}
	if runTrace.TotalTokens == 0 {
		t.Fatalf("expected token usage in trace")
	}

	session := getSession(t, app, resp.SessionKey)
	if session.MessageCount != 2 {
		t.Fatalf("expected message_count=2, got %+v", session)
	}
}

func TestNoReplySuppressed(t *testing.T) {
	app := newTestApp(t)
	req := api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "integration-no-reply", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:integration-no-reply:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "memory_flush", Text: "flush"},
	}
	resp := ingest(t, app, req, http.StatusAccepted)
	event := pollEvent(t, app, resp.EventID)
	if event.Status != model.EventStatusSuppressed {
		t.Fatalf("expected suppressed, got %+v", event)
	}
	if event.OutboxStatus != "" {
		t.Fatalf("expected no outbox status, got %+v", event)
	}
}

func TestNoReplyWritesCanonicalMemoryPaths(t *testing.T) {
	app := newTestApp(t)
	day := time.Now().UTC().Format("2006-01-02")

	flushReq := api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "integration-canonical-dm", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:integration-canonical-dm:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "memory_flush", Text: "flush canonical"},
	}
	flushResp := ingest(t, app, flushReq, http.StatusAccepted)
	flushEvent := pollEvent(t, app, flushResp.EventID)
	if flushEvent.Status != model.EventStatusSuppressed {
		t.Fatalf("expected suppressed flush event, got %+v", flushEvent)
	}
	privateDaily := filepath.Join(app.Cfg.Workspace, "memory", "private", "daily", day+".md")
	privateData, err := os.ReadFile(privateDaily)
	if err != nil {
		t.Fatalf("read private daily: %v", err)
	}
	if !strings.Contains(string(privateData), "flush canonical") {
		t.Fatalf("expected flush text in private daily file, got %q", string(privateData))
	}

	compactReq := api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "integration-canonical-group", ChannelType: "group"},
		IdempotencyKey: "cli:integration-canonical-group:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "compaction", Text: "group canonical"},
	}
	compactResp := ingest(t, app, compactReq, http.StatusAccepted)
	compactEvent := pollEvent(t, app, compactResp.EventID)
	if compactEvent.Status != model.EventStatusSuppressed {
		t.Fatalf("expected suppressed compaction event, got %+v", compactEvent)
	}
	publicCurated := filepath.Join(app.Cfg.Workspace, "memory", "public", "MEMORY.md")
	publicData, err := os.ReadFile(publicCurated)
	if err != nil {
		t.Fatalf("read public curated: %v", err)
	}
	if !strings.Contains(string(publicData), "group canonical") {
		t.Fatalf("expected compaction text in public curated file, got %q", string(publicData))
	}
}

func TestCronFireSuppressedLLMHiddenAndNoLeakToVisibleHistory(t *testing.T) {
	app := newTestAppWithConfig(t, func(cfg *config.Config) {
		cfg.LLM.DefaultModel = "fake/default"
		cfg.LLM.Providers["fake"] = config.LLMProviderConfig{
			Type:                 "fake",
			Timeout:              config.Duration{Duration: 5 * time.Second},
			FakeResponseText:     "roles={{message_roles}} last={{last_user_message}}",
			FakeToolName:         "memory_search",
			FakeToolArgsJSON:     `{"query":"alpha","visibility":"auto","kind":"any","top_k":1}`,
			FakeFinishReason:     "stop",
			FakeRawFinishReason:  "stop",
			FakePromptTokens:     8,
			FakeCompletionTokens: 8,
			FakeRequestID:        "fake-cron-test",
		}
	})
	if err := os.MkdirAll(filepath.Join(app.Cfg.Workspace, "memory", "public"), 0o755); err != nil {
		t.Fatalf("mkdir memory/public: %v", err)
	}
	if err := os.WriteFile(filepath.Join(app.Cfg.Workspace, "memory", "public", "MEMORY.md"), []byte("alpha memory\n"), 0o644); err != nil {
		t.Fatalf("write public memory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(app.Cfg.Workspace, "HEARTBEAT.md"), []byte("- inspect existing memory\n"), 0o644); err != nil {
		t.Fatalf("write HEARTBEAT.md: %v", err)
	}

	conversation := model.Conversation{ConversationID: "integration-cron", ChannelType: "dm", ParticipantID: "u1"}
	cronReq := api.IngestRequest{
		Source:         "cli",
		Conversation:   conversation,
		IdempotencyKey: "cli:integration-cron:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "cron_fire", Text: "nightly heartbeat"},
	}
	cronResp := ingest(t, app, cronReq, http.StatusAccepted)
	cronEvent := pollEvent(t, app, cronResp.EventID)
	if cronEvent.Status != model.EventStatusSuppressed {
		t.Fatalf("expected suppressed cron event, got %+v", cronEvent)
	}
	if cronEvent.AssistantReply != "" || cronEvent.OutboxStatus != "" {
		t.Fatalf("expected no assistant reply or outbox for cron event, got %+v", cronEvent)
	}

	trace := getRunTrace(t, app, cronEvent.RunID)
	if trace.OutputText != "roles=system,user,assistant,tool last=nightly heartbeat" {
		t.Fatalf("expected suppressed cron trace output to reflect tool loop, got %+v", trace)
	}

	visibleHistory := fetchSessionHistory(t, app, cronResp.SessionKey, true)
	if len(visibleHistory.Items) != 0 {
		t.Fatalf("expected default visible history to hide cron messages, got %+v", visibleHistory.Items)
	}

	allHistory := fetchSessionHistory(t, app, cronResp.SessionKey, false)
	if len(allHistory.Items) < 4 {
		t.Fatalf("expected hidden cron chain in full history, got %+v", allHistory.Items)
	}
	foundCronTool := false
	for _, item := range allHistory.Items {
		if item.Visible {
			t.Fatalf("expected cron history items to stay hidden, got %+v", allHistory.Items)
		}
		if item.Meta["payload_type"] != "cron_fire" {
			t.Fatalf("expected cron payload_type meta in history, got %+v", allHistory.Items)
		}
		if item.Role == "tool" && item.ToolName == "memory_search" {
			foundCronTool = true
		}
	}
	if !foundCronTool {
		t.Fatalf("expected hidden cron tool result in history, got %+v", allHistory.Items)
	}

	normalReq := api.IngestRequest{
		Source:         "cli",
		Conversation:   conversation,
		IdempotencyKey: "cli:integration-cron:2",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello after cron"},
	}
	normalResp := ingest(t, app, normalReq, http.StatusAccepted)
	normalEvent := pollEvent(t, app, normalResp.EventID)
	if normalEvent.Status != model.EventStatusProcessed {
		t.Fatalf("expected processed follow-up event, got %+v", normalEvent)
	}
	if normalEvent.AssistantReply != "roles=system,user,assistant,tool last=hello after cron" {
		t.Fatalf("expected follow-up reply without leaked cron history, got %+v", normalEvent)
	}

	visibleAfter := fetchSessionHistory(t, app, cronResp.SessionKey, true)
	if len(visibleAfter.Items) != 3 {
		t.Fatalf("expected only visible follow-up user/tool/assistant messages, got %+v", visibleAfter.Items)
	}
	roles := map[string]bool{}
	for _, item := range visibleAfter.Items {
		roles[item.Role] = true
		if item.Meta["payload_type"] == "cron_fire" {
			t.Fatalf("expected cron history to stay hidden from visible history, got %+v", visibleAfter.Items)
		}
	}
	if !roles["user"] || !roles["assistant"] || !roles["tool"] {
		t.Fatalf("expected follow-up visible history to contain user/tool/assistant only, got %+v", visibleAfter.Items)
	}
}

func TestWorkspacePatchToolUpdatesIdentityFile(t *testing.T) {
	app := newTestAppWithConfig(t, func(cfg *config.Config) {
		cfg.LLM.DefaultModel = "fake/default"
		cfg.LLM.Providers["fake"] = config.LLMProviderConfig{
			Type:                 "fake",
			Timeout:              config.Duration{Duration: 5 * time.Second},
			FakeResponseText:     "done: {{last_user_message}}",
			FakeToolName:         "workspace_patch",
			FakeToolArgsJSON:     `{"path":"IDENTITY.md","old_text":"SimiClaw","new_text":"Simi 龙虾"}`,
			FakeFinishReason:     "stop",
			FakeRawFinishReason:  "stop",
			FakePromptTokens:     8,
			FakeCompletionTokens: 8,
			FakeRequestID:        "fake-workspace-patch",
		}
	})
	if err := os.WriteFile(filepath.Join(app.Cfg.Workspace, "IDENTITY.md"), []byte("- Name: SimiClaw\n"), 0o644); err != nil {
		t.Fatalf("write IDENTITY.md: %v", err)
	}

	req := api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "integration-workspace-patch", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:integration-workspace-patch:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "rename"},
	}
	resp := ingest(t, app, req, http.StatusAccepted)
	event := pollEvent(t, app, resp.EventID)
	if event.Status != model.EventStatusProcessed {
		t.Fatalf("expected processed event, got %+v", event)
	}
	data, err := os.ReadFile(filepath.Join(app.Cfg.Workspace, "IDENTITY.md"))
	if err != nil {
		t.Fatalf("read IDENTITY.md: %v", err)
	}
	if !strings.Contains(string(data), "Simi 龙虾") {
		t.Fatalf("expected patched IDENTITY.md, got %q", string(data))
	}
	history := fetchSessionHistory(t, app, resp.SessionKey, true)
	foundTool := false
	for _, item := range history.Items {
		if item.Role == "tool" && item.ToolName == "workspace_patch" {
			foundTool = true
		}
	}
	if !foundTool {
		t.Fatalf("expected workspace_patch tool message in history, got %+v", history.Items)
	}
}

func TestWorkspaceDeleteToolRemovesBootstrapFile(t *testing.T) {
	app := newTestAppWithConfig(t, func(cfg *config.Config) {
		cfg.LLM.DefaultModel = "fake/default"
		cfg.LLM.Providers["fake"] = config.LLMProviderConfig{
			Type:                 "fake",
			Timeout:              config.Duration{Duration: 5 * time.Second},
			FakeResponseText:     "done: {{last_user_message}}",
			FakeToolName:         "workspace_delete",
			FakeToolArgsJSON:     `{"path":"BOOTSTRAP.md"}`,
			FakeFinishReason:     "stop",
			FakeRawFinishReason:  "stop",
			FakePromptTokens:     8,
			FakeCompletionTokens: 8,
			FakeRequestID:        "fake-workspace-delete",
		}
	})
	if err := os.WriteFile(filepath.Join(app.Cfg.Workspace, "BOOTSTRAP.md"), []byte("cleanup me\n"), 0o644); err != nil {
		t.Fatalf("write BOOTSTRAP.md: %v", err)
	}

	req := api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "integration-workspace-delete", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:integration-workspace-delete:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "cleanup"},
	}
	resp := ingest(t, app, req, http.StatusAccepted)
	event := pollEvent(t, app, resp.EventID)
	if event.Status != model.EventStatusProcessed {
		t.Fatalf("expected processed event, got %+v", event)
	}
	if _, err := os.Stat(filepath.Join(app.Cfg.Workspace, "BOOTSTRAP.md")); !os.IsNotExist(err) {
		t.Fatalf("expected BOOTSTRAP.md deleted, err=%v", err)
	}
	history := fetchSessionHistory(t, app, resp.SessionKey, true)
	foundTool := false
	for _, item := range history.Items {
		if item.Role == "tool" && item.ToolName == "workspace_delete" {
			foundTool = true
		}
	}
	if !foundTool {
		t.Fatalf("expected workspace_delete tool message in history, got %+v", history.Items)
	}
}

func TestNewSessionCommandStartsFreshSession(t *testing.T) {
	app := newTestAppWithConfig(t, func(cfg *config.Config) {
		cfg.LLM.DefaultModel = "fake/default"
		cfg.LLM.Providers["fake"] = config.LLMProviderConfig{
			Type:                 "fake",
			Timeout:              config.Duration{Duration: 5 * time.Second},
			FakeResponseText:     "roles={{message_roles}} last={{last_user_message}}",
			FakeFinishReason:     "stop",
			FakeRawFinishReason:  "stop",
			FakePromptTokens:     8,
			FakeCompletionTokens: 8,
			FakeRequestID:        "fake-new-session",
		}
	})
	conversation := model.Conversation{ConversationID: "integration-new-session", ChannelType: "dm", ParticipantID: "u1"}

	first := ingest(t, app, api.IngestRequest{
		Source:         "cli",
		Conversation:   conversation,
		IdempotencyKey: "cli:integration-new-session:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello first"},
	}, http.StatusAccepted)
	firstEvent := pollEvent(t, app, first.EventID)
	if firstEvent.AssistantReply != "roles=system,user last=hello first" {
		t.Fatalf("unexpected first assistant reply: %+v", firstEvent)
	}

	reset := ingest(t, app, api.IngestRequest{
		Source:         "cli",
		Conversation:   conversation,
		IdempotencyKey: "cli:integration-new-session:2",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "/new"},
	}, http.StatusAccepted)
	resetEvent := pollEvent(t, app, reset.EventID)
	if resetEvent.AssistantReply != "已开始新会话。" {
		t.Fatalf("unexpected reset assistant reply: %+v", resetEvent)
	}
	if resetEvent.SessionKey == firstEvent.SessionKey {
		t.Fatalf("expected /new to create a fresh session key, got %+v and %+v", firstEvent, resetEvent)
	}

	after := ingest(t, app, api.IngestRequest{
		Source:         "cli",
		Conversation:   conversation,
		IdempotencyKey: "cli:integration-new-session:3",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello after new"},
	}, http.StatusAccepted)
	afterEvent := pollEvent(t, app, after.EventID)
	if afterEvent.SessionKey != resetEvent.SessionKey {
		t.Fatalf("expected follow-up message to reuse reset session, got reset=%+v after=%+v", resetEvent, afterEvent)
	}
	if afterEvent.AssistantReply != "roles=system,user last=hello after new" {
		t.Fatalf("expected follow-up reply without leaked history, got %+v", afterEvent)
	}

	newer := ingest(t, app, api.IngestRequest{
		Source:         "cli",
		Conversation:   conversation,
		IdempotencyKey: "cli:integration-new-session:4",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello second after new"},
	}, http.StatusAccepted)
	newerEvent := pollEvent(t, app, newer.EventID)
	if newerEvent.SessionKey != resetEvent.SessionKey {
		t.Fatalf("expected second follow-up to stay in reset session, got reset=%+v newer=%+v", resetEvent, newerEvent)
	}
	if newerEvent.AssistantReply != "roles=system,user,assistant,user last=hello second after new" {
		t.Fatalf("expected second follow-up to see reset-session history, got %+v", newerEvent)
	}

	backToFirst := ingest(t, app, api.IngestRequest{
		Source:         "web",
		Conversation:   conversation,
		SessionKeyHint: first.SessionKey,
		IdempotencyKey: "web:integration-new-session:5",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "back to first"},
	}, http.StatusAccepted)
	backToFirstEvent := pollEvent(t, app, backToFirst.EventID)
	if backToFirstEvent.SessionKey != first.SessionKey {
		t.Fatalf("expected session_key hint to route back to first session, got first=%+v back=%+v", firstEvent, backToFirstEvent)
	}
	if backToFirstEvent.AssistantReply != "roles=system,user,assistant,user last=back to first" {
		t.Fatalf("expected hinted request to use first-session history only, got %+v", backToFirstEvent)
	}
}

func TestReadyzRequiresDBAndEventLoop(t *testing.T) {
	app := newTestApp(t)
	body, code := doRequest(t, app, http.MethodGet, "/readyz", nil)
	if code != http.StatusOK {
		t.Fatalf("readyz expected 200, got %d body=%s", code, string(body))
	}
}

func TestChatStreamAcceptedToDone(t *testing.T) {
	app := newTestApp(t)
	server := httptest.NewServer(app.Handler)
	defer server.Close()

	req := api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "integration-stream", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:integration-stream:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello stream"},
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(server.URL+"/v1/chat:stream", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("chat stream request: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", got)
	}

	reader := bufio.NewReader(resp.Body)
	accepted := readStreamEvent(t, reader)
	if accepted.Type != api.ChatStreamEventAccepted {
		t.Fatalf("expected accepted event, got %+v", accepted)
	}
	var (
		sawText bool
		done    api.ChatStreamEvent
	)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		event := readStreamEvent(t, reader)
		switch event.Type {
		case api.ChatStreamEventTextDelta:
			sawText = true
		case api.ChatStreamEventDone:
			done = event
			goto complete
		}
	}
	t.Fatalf("timeout waiting for done event")

complete:
	if !sawText {
		t.Fatalf("expected at least one text_delta event")
	}
	if done.EventRecord == nil {
		t.Fatalf("done missing event_record: %+v", done)
	}
	if done.EventRecord.EventID != accepted.EventID {
		t.Fatalf("event id mismatch: accepted=%s done=%+v", accepted.EventID, done.EventRecord)
	}
	if done.EventRecord.AssistantReply != "已收到: hello stream" {
		t.Fatalf("unexpected assistant reply: %+v", done.EventRecord)
	}
}

func newTestApp(t *testing.T) *bootstrap.App {
	t.Helper()
	return newTestAppWithConfig(t, nil)
}

func newTestAppWithConfig(t *testing.T, mutate func(*config.Config)) *bootstrap.App {
	t.Helper()
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	if mutate != nil {
		mutate(&cfg)
	}
	if err := store.InitWorkspace(cfg.Workspace, false, cfg.DBBusyTimeout.Duration); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	app, err := bootstrap.NewApp(cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	if err := app.Start(); err != nil {
		t.Fatalf("start app: %v", err)
	}
	t.Cleanup(app.Stop)
	return app
}

func ingest(t *testing.T, app *bootstrap.App, req api.IngestRequest, want int) api.IngestResponse {
	t.Helper()
	body, _ := json.Marshal(req)
	respBody, code := doRequest(t, app, http.MethodPost, "/v1/events:ingest", body)
	if code != want {
		t.Fatalf("expected %d got %d body=%s", want, code, string(respBody))
	}
	var resp api.IngestResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("decode ingest response: %v", err)
	}
	return resp
}

func pollEvent(t *testing.T, app *bootstrap.App, eventID string) model.EventRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body, code := doRequest(t, app, http.MethodGet, "/v1/events/"+eventID, nil)
		if code != http.StatusOK {
			t.Fatalf("event query expected 200, got %d body=%s", code, string(body))
		}
		var event model.EventRecord
		if err := json.Unmarshal(body, &event); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		if event.Status == model.EventStatusSuppressed || event.Status == model.EventStatusFailed {
			return event
		}
		if event.Status == model.EventStatusProcessed && (event.OutboxStatus == model.OutboxStatusSent || event.OutboxStatus == model.OutboxStatusDead) {
			return event
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout polling event %s", eventID)
	return model.EventRecord{}
}

func getRunTrace(t *testing.T, app *bootstrap.App, runID string) model.RunTrace {
	t.Helper()
	body, code := doRequest(t, app, http.MethodGet, "/v1/runs/"+runID+"/trace", nil)
	if code != http.StatusOK {
		t.Fatalf("run trace expected 200, got %d body=%s", code, string(body))
	}
	var trace model.RunTrace
	if err := json.Unmarshal(body, &trace); err != nil {
		t.Fatalf("decode run trace: %v", err)
	}
	return trace
}

func fetchSessionHistory(t *testing.T, app *bootstrap.App, sessionKey string, visibleOnly bool) struct {
	Items []model.MessageRecord `json:"items"`
} {
	t.Helper()
	path := "/v1/sessions/" + sessionKey + "/history"
	if !visibleOnly {
		path += "?visible=false"
	}
	body, code := doRequest(t, app, http.MethodGet, path, nil)
	if code != http.StatusOK {
		t.Fatalf("history query expected 200, got %d body=%s", code, string(body))
	}
	var out struct {
		Items []model.MessageRecord `json:"items"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	return out
}

func getSession(t *testing.T, app *bootstrap.App, sessionKey string) model.SessionRecord {
	t.Helper()
	body, code := doRequest(t, app, http.MethodGet, "/v1/sessions/"+sessionKey, nil)
	if code != http.StatusOK {
		t.Fatalf("session query expected 200, got %d body=%s", code, string(body))
	}
	var session model.SessionRecord
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	return session
}

func doRequest(t *testing.T, app *bootstrap.App, method, path string, body []byte) ([]byte, int) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	app.Handler.ServeHTTP(rr, req)
	return rr.Body.Bytes(), rr.Code
}

func readStreamEvent(t *testing.T, reader *bufio.Reader) api.ChatStreamEvent {
	t.Helper()
	var (
		eventType string
		data      []byte
	)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE line: %v", err)
		}
		if line == "\n" {
			if eventType == "" && len(data) == 0 {
				continue
			}
			var event api.ChatStreamEvent
			if err := json.Unmarshal(data, &event); err != nil {
				t.Fatalf("decode SSE payload: %v", err)
			}
			if string(event.Type) != eventType {
				t.Fatalf("event type mismatch header=%s payload=%s", eventType, event.Type)
			}
			return event
		}
		if len(line) > 0 && line[0] == ':' {
			continue
		}
		switch {
		case len(line) > len("event: ") && line[:len("event: ")] == "event: ":
			eventType = trimLine(line[len("event: "):])
		case len(line) > len("data: ") && line[:len("data: ")] == "data: ":
			if data == nil {
				data = []byte(trimLine(line[len("data: "):]))
				continue
			}
			data = append(data, '\n')
			data = append(data, trimLine(line[len("data: "):])...)
		}
	}
}

func trimLine(in string) string {
	for len(in) > 0 && (in[len(in)-1] == '\n' || in[len(in)-1] == '\r') {
		in = in[:len(in)-1]
	}
	return in
}
