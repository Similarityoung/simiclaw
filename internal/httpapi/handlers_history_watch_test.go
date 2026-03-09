package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/bootstrap"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestSessionHistoryVisibleFilterAndPagination(t *testing.T) {
	app, srv := newHTTPAPITestServer(t)
	conversation := "history_test"
	for i := 1; i <= 3; i++ {
		eventID := ingestTestEvent(t, srv.URL, buildCLIRequest(conversation, i, "message", fmt.Sprintf("hello %d", i)))
		waitEventTerminal(t, app, eventID)
	}
	compactionEventID := ingestTestEvent(t, srv.URL, buildCLIRequest(conversation, 99, "compaction", "compact once"))
	waitEventTerminal(t, app, compactionEventID)

	sessionKey := findSessionKeyByConversation(t, app, conversation)
	page := fetchHistoryPage(t, srv.URL, sessionKey, "", 2, true)
	if len(page.Items) != 2 {
		t.Fatalf("visible history len=%d want=2", len(page.Items))
	}
	if page.NextCursor == "" {
		t.Fatal("expected next_cursor for paginated history")
	}
	for _, item := range page.Items {
		if !item.Visible {
			t.Fatalf("expected visible item, got invisible %#v", item)
		}
	}

	nextPage := fetchHistoryPage(t, srv.URL, sessionKey, page.NextCursor, 2, true)
	if len(nextPage.Items) == 0 {
		t.Fatal("expected older visible messages on next page")
	}

	allPage := fetchHistoryPage(t, srv.URL, sessionKey, "", 16, false)
	if len(allPage.Items) != 7 {
		t.Fatalf("all history len=%d want=7", len(allPage.Items))
	}
	foundHidden := false
	for _, item := range allPage.Items {
		if !item.Visible {
			foundHidden = true
		}
	}
	if !foundHidden {
		t.Fatal("expected hidden message when visible=false")
	}
}

func newHTTPAPITestServer(t *testing.T) (*bootstrap.App, *httptest.Server) {
	t.Helper()
	workspace := t.TempDir()
	if err := store.InitWorkspace(workspace, false, config.Default().DBBusyTimeout.Duration); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.LLM.Providers["fake"] = config.LLMProviderConfig{
		Type:                 "fake",
		Timeout:              config.Duration{Duration: 5 * time.Second},
		FakeResponseText:     strings.Repeat("已收到: {{last_user_message}}", 2),
		FakeFinishReason:     "stop",
		FakeRawFinishReason:  "stop",
		FakePromptTokens:     8,
		FakeCompletionTokens: 8,
		FakeRequestID:        "fake-stream-test",
	}
	app, err := bootstrap.NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if err := app.Start(); err != nil {
		t.Fatalf("start app: %v", err)
	}
	t.Cleanup(app.Stop)
	srv := httptest.NewServer(app.Handler)
	t.Cleanup(srv.Close)
	return app, srv
}

func buildCLIRequest(conversation string, seq int, payloadType, text string) model.IngestRequest {
	return model.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: conversation,
			ChannelType:    "dm",
			ParticipantID:  "local_user",
		},
		IdempotencyKey: fmt.Sprintf("cli:%s:%d", conversation, seq),
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload: model.EventPayload{
			Type: payloadType,
			Text: text,
		},
	}
}

func ingestTestEvent(t *testing.T, baseURL string, req model.IngestRequest) string {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := http.Post(baseURL+"/v1/events:ingest", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST ingest: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("ingest status=%d body=%s", resp.StatusCode, string(data))
	}
	var out model.IngestResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode ingest response: %v", err)
	}
	return out.EventID
}

func waitEventTerminal(t *testing.T, app *bootstrap.App, eventID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec, ok, err := app.DB.GetEvent(context.Background(), eventID)
		if err != nil {
			t.Fatalf("GetEvent: %v", err)
		}
		if ok {
			switch rec.Status {
			case model.EventStatusProcessed, model.EventStatusSuppressed, model.EventStatusFailed:
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("event %s did not reach terminal state", eventID)
}

func findSessionKeyByConversation(t *testing.T, app *bootstrap.App, conversation string) string {
	t.Helper()
	sessions, err := app.DB.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	for _, item := range sessions {
		if item.ConversationID == conversation {
			return item.SessionKey
		}
	}
	t.Fatalf("session for conversation %s not found", conversation)
	return ""
}

func fetchHistoryPage(t *testing.T, baseURL, sessionKey, cursor string, limit int, visibleOnly bool) struct {
	Items      []model.MessageRecord `json:"items"`
	NextCursor string                `json:"next_cursor"`
} {
	t.Helper()
	url := fmt.Sprintf("%s/v1/sessions/%s/history?limit=%d", baseURL, sessionKey, limit)
	if cursor != "" {
		url += "&cursor=" + cursor
	}
	if !visibleOnly {
		url += "&visible=false"
	}
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET history: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("history status=%d body=%s", resp.StatusCode, string(body))
	}
	var out struct {
		Items      []model.MessageRecord `json:"items"`
		NextCursor string                `json:"next_cursor"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	return out
}
