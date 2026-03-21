//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/bootstrap"
	telegramchannel "github.com/similarityyoung/simiclaw/internal/channels/telegram"
	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/gateway"
	gatewaybindings "github.com/similarityyoung/simiclaw/internal/gateway/bindings"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/internal/store"
	storequeries "github.com/similarityyoung/simiclaw/internal/store/queries"
	storetx "github.com/similarityyoung/simiclaw/internal/store/tx"
	apitypes "github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
	tele "gopkg.in/telebot.v4"
)

type fakeTelegramAPIServer struct {
	t      *testing.T
	token  string
	server *httptest.Server

	mu                 sync.Mutex
	updates            []tele.Update
	sent               []fakeSentMessage
	deleteWebhookCalls int
	getMeCalls         int
}

type fakeSentMessage struct {
	ChatID string
	Text   string
}

func newFakeTelegramAPIServer(t *testing.T, token string) *fakeTelegramAPIServer {
	t.Helper()
	api := &fakeTelegramAPIServer{t: t, token: token}
	api.server = httptest.NewServer(http.HandlerFunc(api.handle))
	t.Cleanup(api.server.Close)
	return api
}

func (f *fakeTelegramAPIServer) URL() string {
	return f.server.URL
}

func (f *fakeTelegramAPIServer) Client() *http.Client {
	return f.server.Client()
}

func (f *fakeTelegramAPIServer) Enqueue(update tele.Update) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, update)
}

func (f *fakeTelegramAPIServer) WaitSent(t *testing.T, count int) fakeSentMessage {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		if len(f.sent) >= count {
			msg := f.sent[count-1]
			f.mu.Unlock()
			return msg
		}
		f.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for fake telegram sendMessage #%d", count)
	return fakeSentMessage{}
}

func (f *fakeTelegramAPIServer) handle(w http.ResponseWriter, r *http.Request) {
	prefix := "/bot" + f.token + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	method := strings.TrimPrefix(r.URL.Path, prefix)
	switch method {
	case "getMe":
		f.mu.Lock()
		f.getMeCalls++
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"id": 9000, "is_bot": true, "first_name": "Simi", "username": "simiclaw_bot"}})
	case "deleteWebhook":
		f.mu.Lock()
		f.deleteWebhookCalls++
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
	case "getUpdates":
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			f.t.Fatalf("decode getUpdates payload: %v", err)
		}
		offset, _ := strconv.Atoi(payload["offset"])
		f.mu.Lock()
		updates := make([]tele.Update, 0, len(f.updates))
		for _, update := range f.updates {
			if update.ID >= offset {
				updates = append(updates, update)
			}
		}
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": updates})
	case "sendMessage":
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			f.t.Fatalf("decode sendMessage payload: %v", err)
		}
		f.mu.Lock()
		f.sent = append(f.sent, fakeSentMessage{ChatID: payload["chat_id"], Text: payload["text"]})
		messageID := len(f.sent)
		f.mu.Unlock()
		chatID, _ := strconv.ParseInt(payload["chat_id"], 10, 64)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"message_id": messageID, "date": time.Now().UTC().Unix(), "text": payload["text"], "chat": map[string]any{"id": chatID, "type": "private"}}})
	default:
		http.NotFound(w, r)
	}
}

func TestTelegramPendingOutboxIsDeliveredOnStartup(t *testing.T) {
	const token = "telegram-test-token-pending"
	api := newFakeTelegramAPIServer(t, token)
	restore := telegramchannel.SetRuntimeTestHooksForTesting(api.URL(), api.Client())
	defer restore()

	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Telegram.Token = token
	cfg.Channels.Telegram.AllowedUserIDs = []int64{1001}
	cfg.Channels.Telegram.LongPollTimeout = config.Duration{Duration: 50 * time.Millisecond}
	if err := store.InitWorkspace(workspace, false, cfg.DBBusyTimeout.Duration); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}
	db, err := store.Open(workspace, cfg.DBBusyTimeout.Duration)
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}
	conversation := model.Conversation{ConversationID: "tg_chat_3003", ChannelType: "dm", ParticipantID: "1001"}
	sessionKey, err := gatewaybindings.ComputeKey(cfg.TenantID, conversation, "default")
	if err != nil {
		t.Fatalf("ComputeKey: %v", err)
	}
	repo := storetx.NewRuntimeRepository(db)
	now := time.Now().UTC()
	req := apitypes.IngestRequest{
		Source:         "telegram",
		Conversation:   conversation,
		IdempotencyKey: "telegram:update:301",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload: model.EventPayload{
			Type: "message",
			Text: "seed inbound",
			Extra: map[string]string{
				"telegram_chat_id":        "3003",
				"telegram_message_id":     "401",
				"telegram_update_id":      "301",
				"telegram_participant_id": "1001",
			},
		},
	}
	result, err := repo.PersistEvent(context.Background(), cfg.TenantID, sessionKey, gateway.PersistRequest{
		Source:         req.Source,
		Conversation:   req.Conversation,
		Payload:        req.Payload,
		IdempotencyKey: req.IdempotencyKey,
		DMScope:        req.DMScope,
	}, fmt.Sprintf("seed:%d", now.UnixNano()), now)
	if err != nil {
		t.Fatalf("PersistEvent: %v", err)
	}
	if err := repo.MarkEventQueued(context.Background(), result.EventID, now); err != nil {
		t.Fatalf("MarkEventQueued: %v", err)
	}
	claimed, ok, err := repo.ClaimWork(context.Background(), runtimemodel.WorkItem{
		EventID: result.EventID,
	}, "run_seed_1", now)
	if err != nil || !ok {
		t.Fatalf("ClaimWork ok=%v err=%v", ok, err)
	}
	if err := repo.Finalize(context.Background(), runtimemodel.RunFinalize{
		RunID:          claimed.RunID,
		EventID:        claimed.Event.EventID,
		SessionKey:     claimed.Event.SessionKey,
		SessionID:      claimed.Event.ActiveSessionID,
		RunMode:        model.RunModeNormal,
		RunStatus:      model.RunStatusCompleted,
		EventStatus:    model.EventStatusProcessed,
		AssistantReply: "queued telegram reply",
		OutboxChannel:  "telegram",
		OutboxTargetID: "3003",
		OutboxBody:     "queued telegram reply",
		Now:            now,
	}); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close DB: %v", err)
	}

	app, err := bootstrap.NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if err := app.Start(context.Background()); err != nil {
		t.Fatalf("Start app: %v", err)
	}
	t.Cleanup(app.Stop)

	waitReadyzTelegram(t, app)
	msg := api.WaitSent(t, 1)
	if msg.ChatID != "3003" || msg.Text != "queued telegram reply" {
		t.Fatalf("unexpected delivered startup outbox: %+v", msg)
	}
	event := pollEvent(t, app, result.EventID)
	if event.OutboxStatus != model.OutboxStatusSent {
		t.Fatalf("expected sent outbox after startup recovery, got %+v", event)
	}
}

func TestTelegramPrivateMessageEndToEnd(t *testing.T) {
	const token = "telegram-test-token"
	api := newFakeTelegramAPIServer(t, token)
	restore := telegramchannel.SetRuntimeTestHooksForTesting(api.URL(), api.Client())
	defer restore()

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
			FakeRequestID:        "fake-telegram-message",
		}
		cfg.Channels.Telegram.Enabled = true
		cfg.Channels.Telegram.Token = token
		cfg.Channels.Telegram.AllowedUserIDs = []int64{1001}
		cfg.Channels.Telegram.LongPollTimeout = config.Duration{Duration: 50 * time.Millisecond}
	})

	waitReadyzTelegram(t, app)
	api.Enqueue(tele.Update{
		ID: 100,
		Message: &tele.Message{
			ID:       200,
			Unixtime: time.Now().Add(-2 * time.Hour).Unix(),
			Text:     "hello telegram",
			Chat:     &tele.Chat{ID: 3001, Type: tele.ChatPrivate},
			Sender:   &tele.User{ID: 1001},
		},
	})

	eventID := waitTelegramEventID(t, app, "telegram:update:100")
	event := pollEvent(t, app, eventID)
	if event.AssistantReply != "roles=system,user last=hello telegram" {
		t.Fatalf("unexpected assistant reply: %+v", event)
	}
	msg := api.WaitSent(t, 1)
	if msg.ChatID != "3001" {
		t.Fatalf("unexpected telegram target chat: %+v", msg)
	}
	if msg.Text != "roles=system,user last=hello telegram" {
		t.Fatalf("unexpected telegram text: %+v", msg)
	}
}

func TestTelegramNewCommandStartsFreshSession(t *testing.T) {
	const token = "telegram-test-token-new"
	api := newFakeTelegramAPIServer(t, token)
	restore := telegramchannel.SetRuntimeTestHooksForTesting(api.URL(), api.Client())
	defer restore()

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
			FakeRequestID:        "fake-telegram-new-session",
		}
		cfg.Channels.Telegram.Enabled = true
		cfg.Channels.Telegram.Token = token
		cfg.Channels.Telegram.AllowedUserIDs = []int64{1001}
		cfg.Channels.Telegram.LongPollTimeout = config.Duration{Duration: 50 * time.Millisecond}
	})

	waitReadyzTelegram(t, app)
	api.Enqueue(tele.Update{ID: 110, Message: &tele.Message{ID: 210, Unixtime: time.Now().UTC().Unix(), Text: "hello first", Chat: &tele.Chat{ID: 3010, Type: tele.ChatPrivate}, Sender: &tele.User{ID: 1001}}})
	firstEvent := pollEvent(t, app, waitTelegramEventID(t, app, "telegram:update:110"))
	if firstEvent.AssistantReply != "roles=system,user last=hello first" {
		t.Fatalf("unexpected first assistant reply: %+v", firstEvent)
	}
	if msg := api.WaitSent(t, 1); msg.Text != "roles=system,user last=hello first" {
		t.Fatalf("unexpected first telegram text: %+v", msg)
	}

	api.Enqueue(tele.Update{ID: 111, Message: &tele.Message{ID: 211, Unixtime: time.Now().UTC().Unix(), Text: "/new", Chat: &tele.Chat{ID: 3010, Type: tele.ChatPrivate}, Sender: &tele.User{ID: 1001}}})
	resetEvent := pollEvent(t, app, waitTelegramEventID(t, app, "telegram:update:111"))
	if resetEvent.AssistantReply != "已开始新会话。" {
		t.Fatalf("unexpected reset assistant reply: %+v", resetEvent)
	}
	if resetEvent.SessionKey == firstEvent.SessionKey {
		t.Fatalf("expected telegram /new to create a fresh session, got first=%+v reset=%+v", firstEvent, resetEvent)
	}
	if msg := api.WaitSent(t, 2); msg.Text != "已开始新会话。" {
		t.Fatalf("unexpected reset telegram text: %+v", msg)
	}

	api.Enqueue(tele.Update{ID: 112, Message: &tele.Message{ID: 212, Unixtime: time.Now().UTC().Unix(), Text: "hello after new", Chat: &tele.Chat{ID: 3010, Type: tele.ChatPrivate}, Sender: &tele.User{ID: 1001}}})
	afterEvent := pollEvent(t, app, waitTelegramEventID(t, app, "telegram:update:112"))
	if afterEvent.SessionKey != resetEvent.SessionKey {
		t.Fatalf("expected follow-up telegram message to reuse reset session, got reset=%+v after=%+v", resetEvent, afterEvent)
	}
	if afterEvent.AssistantReply != "roles=system,user last=hello after new" {
		t.Fatalf("unexpected follow-up assistant reply: %+v", afterEvent)
	}
	if msg := api.WaitSent(t, 3); msg.Text != "roles=system,user last=hello after new" {
		t.Fatalf("unexpected follow-up telegram text: %+v", msg)
	}
}

func TestTelegramRejectsUserOutsideAllowlist(t *testing.T) {
	const token = "telegram-test-token-blocked"
	api := newFakeTelegramAPIServer(t, token)
	restore := telegramchannel.SetRuntimeTestHooksForTesting(api.URL(), api.Client())
	defer restore()

	app := newTestAppWithConfig(t, func(cfg *config.Config) {
		cfg.Channels.Telegram.Enabled = true
		cfg.Channels.Telegram.Token = token
		cfg.Channels.Telegram.AllowedUserIDs = []int64{1001}
		cfg.Channels.Telegram.LongPollTimeout = config.Duration{Duration: 50 * time.Millisecond}
	})

	api.Enqueue(tele.Update{
		ID: 101,
		Message: &tele.Message{
			ID:       201,
			Unixtime: time.Now().UTC().Unix(),
			Text:     "blocked user",
			Chat:     &tele.Chat{ID: 3002, Type: tele.ChatPrivate},
			Sender:   &tele.User{ID: 2002},
		},
	})
	waitNoTelegramEvent(t, app, "telegram:update:101")
}

func TestTelegramIgnoresGroupMessage(t *testing.T) {
	const token = "telegram-test-token-group"
	api := newFakeTelegramAPIServer(t, token)
	restore := telegramchannel.SetRuntimeTestHooksForTesting(api.URL(), api.Client())
	defer restore()

	app := newTestAppWithConfig(t, func(cfg *config.Config) {
		cfg.Channels.Telegram.Enabled = true
		cfg.Channels.Telegram.Token = token
		cfg.Channels.Telegram.AllowedUserIDs = []int64{1001}
		cfg.Channels.Telegram.LongPollTimeout = config.Duration{Duration: 50 * time.Millisecond}
	})

	api.Enqueue(tele.Update{
		ID: 102,
		Message: &tele.Message{
			ID:       202,
			Unixtime: time.Now().UTC().Unix(),
			Text:     "hello group",
			Chat:     &tele.Chat{ID: -4001, Type: tele.ChatGroup},
			Sender:   &tele.User{ID: 1001},
		},
	})
	waitNoTelegramEvent(t, app, "telegram:update:102")
}

func waitReadyzTelegram(t *testing.T, app *bootstrap.App) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body, code := doRequest(t, app, http.MethodGet, "/readyz", nil)
		if code == http.StatusOK {
			var state map[string]any
			if err := json.Unmarshal(body, &state); err != nil {
				t.Fatalf("decode readyz: %v", err)
			}
			if state["telegram_polling"] == "alive" {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timeout waiting for readyz telegram_polling=alive")
}

func waitTelegramEventID(t *testing.T, app *bootstrap.App, key string) string {
	t.Helper()
	queryRepo := storequeries.NewRepository(app.DB)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		lookup, ok, err := queryRepo.LookupEvent(context.Background(), key)
		if err != nil {
			t.Fatalf("LookupEvent: %v", err)
		}
		if ok {
			return lookup.EventID
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for telegram event %s", key)
	return ""
}

func waitNoTelegramEvent(t *testing.T, app *bootstrap.App, key string) {
	t.Helper()
	queryRepo := storequeries.NewRepository(app.DB)
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		_, ok, err := queryRepo.LookupEvent(context.Background(), key)
		if err != nil {
			t.Fatalf("LookupEvent: %v", err)
		}
		if ok {
			t.Fatalf("unexpected telegram event for %s", key)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
