package httpapi_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/bootstrap"
	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/api"
)

func TestRunsAndSessionsPaginationAndValidation(t *testing.T) {
	app, srv := newHTTPAPITestServer(t)
	for i, conversation := range []string{"runs_sessions_a", "runs_sessions_b"} {
		eventID := ingestTestEvent(t, srv.URL, buildCLIRequest(conversation, i+1, "message", fmt.Sprintf("hello %d", i+1)))
		waitEventTerminal(t, app, eventID)
	}

	sessionPage1 := fetchJSON[struct {
		Items      []api.SessionRecord `json:"items"`
		NextCursor string              `json:"next_cursor"`
	}](t, srv.URL+"/v1/sessions?limit=1")
	if len(sessionPage1.Items) != 1 || sessionPage1.NextCursor == "" {
		t.Fatalf("expected one session with next cursor, got %+v", sessionPage1)
	}
	sessionPage2 := fetchJSON[struct {
		Items      []api.SessionRecord `json:"items"`
		NextCursor string              `json:"next_cursor"`
	}](t, srv.URL+"/v1/sessions?limit=1&cursor="+url.QueryEscape(sessionPage1.NextCursor))
	if len(sessionPage2.Items) != 1 {
		t.Fatalf("expected one session on second page, got %+v", sessionPage2)
	}
	if sessionPage1.Items[0].SessionKey == sessionPage2.Items[0].SessionKey {
		t.Fatalf("expected different sessions across pages, got %+v %+v", sessionPage1.Items, sessionPage2.Items)
	}

	filteredSessions := fetchJSON[struct {
		Items []api.SessionRecord `json:"items"`
	}](t, srv.URL+"/v1/sessions?conversation_id=runs_sessions_a&limit=10")
	if len(filteredSessions.Items) != 1 {
		t.Fatalf("expected one filtered session, got %+v", filteredSessions.Items)
	}

	invalidSessionCursor := mustEncodeCursor(t, map[string]any{
		"v":                1,
		"last_activity_at": "bad-time",
		"last_session_key": "sess_1",
	})
	getStatus(t, srv.URL+"/v1/sessions?cursor="+url.QueryEscape(invalidSessionCursor), http.StatusBadRequest)

	runPage1 := fetchJSON[struct {
		Items      []map[string]any `json:"items"`
		NextCursor string           `json:"next_cursor"`
	}](t, srv.URL+"/v1/runs?limit=1")
	if len(runPage1.Items) != 1 || runPage1.NextCursor == "" {
		t.Fatalf("expected one run with next cursor, got %+v", runPage1)
	}
	runPage2 := fetchJSON[struct {
		Items []map[string]any `json:"items"`
	}](t, srv.URL+"/v1/runs?limit=1&cursor="+url.QueryEscape(runPage1.NextCursor))
	if len(runPage2.Items) != 1 {
		t.Fatalf("expected one run on second page, got %+v", runPage2)
	}
	if runPage1.Items[0]["run_id"] == runPage2.Items[0]["run_id"] {
		t.Fatalf("expected different runs across pages, got %+v %+v", runPage1.Items, runPage2.Items)
	}

	sessionKey := filteredSessions.Items[0].SessionKey
	filteredRunsBySessionKey := fetchJSON[struct {
		Items []map[string]any `json:"items"`
	}](t, srv.URL+"/v1/runs?session_key="+url.QueryEscape(sessionKey)+"&limit=10")
	if len(filteredRunsBySessionKey.Items) != 1 {
		t.Fatalf("expected one run for session_key, got %+v", filteredRunsBySessionKey.Items)
	}
	sessionID, _ := filteredRunsBySessionKey.Items[0]["session_id"].(string)
	if sessionID == "" {
		t.Fatalf("expected session_id in run summary, got %+v", filteredRunsBySessionKey.Items[0])
	}
	filteredRunsBySessionID := fetchJSON[struct {
		Items []map[string]any `json:"items"`
	}](t, srv.URL+"/v1/runs?session_id="+url.QueryEscape(sessionID)+"&limit=10")
	if len(filteredRunsBySessionID.Items) != 1 {
		t.Fatalf("expected one run for session_id, got %+v", filteredRunsBySessionID.Items)
	}
	if filteredRunsBySessionID.Items[0]["run_id"] != filteredRunsBySessionKey.Items[0]["run_id"] {
		t.Fatalf("expected session_id filter to return same run, key=%+v id=%+v", filteredRunsBySessionKey.Items, filteredRunsBySessionID.Items)
	}

	invalidRunCursor := mustEncodeCursor(t, map[string]any{
		"v":               1,
		"last_started_at": "bad-time",
		"last_run_id":     "run_1",
	})
	getStatus(t, srv.URL+"/v1/runs?cursor="+url.QueryEscape(invalidRunCursor), http.StatusBadRequest)
	getStatus(t, srv.URL+"/v1/runs/missing", http.StatusNotFound)
}

func TestLookupHistoryAndReadyzErrorBranches(t *testing.T) {
	app, srv := newHTTPAPITestServer(t)
	eventID := ingestTestEvent(t, srv.URL, buildCLIRequest("history_errors", 1, "message", "hello history"))
	waitEventTerminal(t, app, eventID)

	sessionKey := findSessionKeyByConversation(t, app, "history_errors")
	getStatus(t, srv.URL+"/v1/events:lookup?idempotency_key=missing", http.StatusNotFound)
	getStatus(t, fmt.Sprintf("%s/v1/sessions/%s/history?visible=maybe", srv.URL, sessionKey), http.StatusBadRequest)

	invalidHistoryCursor := mustEncodeCursor(t, map[string]any{
		"v":               1,
		"last_created_at": "bad-time",
		"last_message_id": "msg_1",
	})
	getStatus(t, fmt.Sprintf("%s/v1/sessions/%s/history?cursor=%s", srv.URL, sessionKey, url.QueryEscape(invalidHistoryCursor)), http.StatusBadRequest)
	getStatus(t, srv.URL+"/v1/sessions/missing/history", http.StatusNotFound)

	readySrv := newUnstartedHTTPAPITestServer(t)
	resp, err := http.Get(readySrv.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET readyz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 503 readyz, got %d body=%s", resp.StatusCode, string(body))
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode readyz body: %v", err)
	}
	if body["status"] != "not_ready" || body["event_loop"] != "down" {
		t.Fatalf("unexpected readyz body: %+v", body)
	}
}

func newUnstartedHTTPAPITestServer(t *testing.T) *httptest.Server {
	t.Helper()
	workspace := t.TempDir()
	cfg := config.Default()
	if err := store.InitWorkspace(workspace, false, cfg.DBBusyTimeout.Duration); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}
	cfg.Workspace = workspace
	cfg.LLM.Providers["fake"] = config.LLMProviderConfig{
		Type:                 "fake",
		Timeout:              config.Duration{Duration: 5 * time.Second},
		FakeResponseText:     "readyz",
		FakeFinishReason:     "stop",
		FakeRawFinishReason:  "stop",
		FakePromptTokens:     1,
		FakeCompletionTokens: 1,
		FakeRequestID:        "fake-readyz-test",
	}
	app, err := bootstrap.NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(app.Stop)
	srv := httptest.NewServer(app.Handler)
	t.Cleanup(srv.Close)
	return srv
}

func mustEncodeCursor(t *testing.T, v any) string {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal cursor: %v", err)
	}
	return base64.StdEncoding.EncodeToString(body)
}
