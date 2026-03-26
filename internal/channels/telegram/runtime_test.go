package telegram

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/gateway"
	gatewaybindings "github.com/similarityyoung/simiclaw/internal/gateway/bindings"
	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
	gatewayrouting "github.com/similarityyoung/simiclaw/internal/gateway/routing"
	runtimepayload "github.com/similarityyoung/simiclaw/internal/runtime/payload"
	"github.com/similarityyoung/simiclaw/internal/testutil/logcapture"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
	tele "gopkg.in/telebot.v4"
)

type telegramRepoStub struct {
	result gateway.PersistResult
	err    error
}

func (r *telegramRepoStub) PersistEvent(context.Context, string, string, gateway.PersistRequest, string, time.Time) (gateway.PersistResult, error) {
	return r.result, r.err
}

func (r *telegramRepoStub) MarkEventQueued(context.Context, string, time.Time) error {
	return nil
}

func (r *telegramRepoStub) GetConversationDMScope(context.Context, string, model.Conversation) (string, bool, error) {
	return "", false, nil
}

func (r *telegramRepoStub) GetScopeSession(context.Context, string) (gatewaybindings.SessionScopeRecord, bool, error) {
	return gatewaybindings.SessionScopeRecord{}, false, nil
}

type telegramQueueStub struct{}

func (telegramQueueStub) TryEnqueue(string) bool { return true }

type telegramRejectQueueStub struct{}

func (telegramRejectQueueStub) TryEnqueue(string) bool { return false }

type captureTelegramGateway struct {
	got      gatewaymodel.NormalizedIngress
	accepted gateway.AcceptedIngest
}

func (g *captureTelegramGateway) Accept(_ context.Context, in gatewaymodel.NormalizedIngress) (gateway.AcceptedIngest, *gateway.APIError) {
	g.got = in
	return g.accepted, nil
}

type fakeHeartbeatRecorder struct {
	mu     sync.Mutex
	names  []string
	fail   error
	notify chan struct{}
}

func (r *fakeHeartbeatRecorder) BeatHeartbeat(_ context.Context, name string, _ time.Time) error {
	r.mu.Lock()
	r.names = append(r.names, name)
	notify := r.notify
	fail := r.fail
	r.mu.Unlock()
	if notify != nil {
		select {
		case notify <- struct{}{}:
		default:
		}
	}
	return fail
}

func (r *fakeHeartbeatRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.names)
}

func (r *fakeHeartbeatRecorder) firstName() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.names) == 0 {
		return ""
	}
	return r.names[0]
}

func TestRunHeartbeatBeatsImmediatelyAndPeriodically(t *testing.T) {
	recorder := &fakeHeartbeatRecorder{notify: make(chan struct{}, 8)}
	runtime := &Runtime{
		heartbeat:         recorder,
		heartbeatInterval: 10 * time.Millisecond,
		logger:            logging.L("telegram-test"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	runtime.wg.Add(1)
	go runtime.runHeartbeat(ctx)
	defer func() {
		cancel()
		runtime.wg.Wait()
	}()

	waitForHeartbeatCount(t, recorder, 1)
	if got := recorder.firstName(); got != "telegram_polling" {
		t.Fatalf("expected heartbeat name telegram_polling, got %q", got)
	}
	waitForHeartbeatCount(t, recorder, 2)
}

func TestRunHeartbeatErrorsDoNotStopLoop(t *testing.T) {
	recorder := &fakeHeartbeatRecorder{
		fail:   errors.New("boom"),
		notify: make(chan struct{}, 8),
	}
	runtime := &Runtime{
		heartbeat:         recorder,
		heartbeatInterval: 10 * time.Millisecond,
		logger:            logging.L("telegram-test"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	runtime.wg.Add(1)
	go runtime.runHeartbeat(ctx)
	defer func() {
		cancel()
		runtime.wg.Wait()
	}()

	waitForHeartbeatCount(t, recorder, 2)
}

func TestRunHeartbeatLogsFailure(t *testing.T) {
	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		recorder := &fakeHeartbeatRecorder{
			fail:   errors.New("boom"),
			notify: make(chan struct{}, 8),
		}
		runtime := &Runtime{
			heartbeat:         recorder,
			heartbeatInterval: 10 * time.Millisecond,
			logger:            logging.L("telegram"),
		}
		ctx, cancel := context.WithCancel(context.Background())
		runtime.wg.Add(1)
		go runtime.runHeartbeat(ctx)
		waitForHeartbeatCount(t, recorder, 1)
		cancel()
		runtime.wg.Wait()
		_ = logging.Sync()
	})

	line := findLogLine(out, "[telegram] telegram heartbeat failed")
	if line == "" {
		t.Fatalf("expected heartbeat failure log, got %q", out)
	}
	if !strings.Contains(line, "\tWARN\t") {
		t.Fatalf("expected WARN level log, got %q", line)
	}
}

func TestRunHeartbeatReturnsImmediatelyWithoutRecorder(t *testing.T) {
	runtime := &Runtime{
		heartbeat:         nil,
		heartbeatInterval: 10 * time.Millisecond,
		logger:            logging.L("telegram-test"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	runtime.wg.Add(1)
	go func() {
		defer close(done)
		runtime.runHeartbeat(ctx)
	}()
	defer cancel()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected runHeartbeat to return immediately when recorder is nil")
	}
	runtime.wg.Wait()
}

func TestRuntimeStartStopLogsLifecycle(t *testing.T) {
	server := newTelegramAPIServer(t)
	defer server.Close()
	restore := SetRuntimeTestHooksForTesting(server.URL, server.Client())
	defer restore()

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		runtime, err := NewRuntime(config.TelegramChannelConfig{
			Token:           "test-token",
			LongPollTimeout: config.Duration{Duration: 10 * time.Millisecond},
		}, nil, nil)
		if err != nil {
			t.Fatalf("NewRuntime: %v", err)
		}
		if err := runtime.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
		runtime.Stop()
		_ = logging.Sync()
	})

	logcapture.AssertContainsInOrder(t, out,
		"[telegram] telegram runtime starting",
		"[telegram] telegram runtime started",
		"[telegram] telegram runtime stopping",
		"[telegram] telegram runtime stopped",
	)
}

func TestHandleTextLogsIgnoredUpdateReason(t *testing.T) {
	ctx := tele.NewContext(nil, tele.Update{
		ID: 42,
		Message: &tele.Message{
			ID:   7,
			Text: "hello",
			Chat: &tele.Chat{ID: 100, Type: tele.ChatPrivate},
			Sender: &tele.User{
				ID: 1001,
			},
		},
	})

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		runtime := &Runtime{
			allowedUsers: map[int64]struct{}{},
			logger:       logging.L("telegram"),
		}
		if err := runtime.handleText(ctx); err != nil {
			t.Fatalf("handleText: %v", err)
		}
		_ = logging.Sync()
	})

	line := findLogLine(out, "[telegram] telegram update ignored")
	if line == "" {
		t.Fatalf("expected telegram update ignored log in %q", out)
	}
	if !strings.Contains(line, "\tINFO\t") {
		t.Fatalf("expected INFO level log, got %q", line)
	}
	for _, part := range []string{
		`"reason": "user_not_allowed"`,
		`"update_id": 42`,
	} {
		if !strings.Contains(line, part) {
			t.Fatalf("missing %q in %q", part, line)
		}
	}
}

func TestHandleTextLogsIngestAccepted(t *testing.T) {
	now := time.Now().UTC()
	ctx := tele.NewContext(nil, tele.Update{
		ID: 44,
		Message: &tele.Message{
			ID:   9,
			Text: "hello",
			Chat: &tele.Chat{ID: 100, Type: tele.ChatPrivate},
			Sender: &tele.User{
				ID: 1001,
			},
		},
	})

	repo := &telegramRepoStub{
		result: gateway.PersistResult{
			EventID:     "evt_accepted",
			SessionKey:  "local:dm:telegram",
			SessionID:   "ses_accepted",
			ReceivedAt:  now,
			PayloadHash: "sha256:test",
		},
	}

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		svc := gateway.NewService(
			"local",
			repo,
			telegramQueueStub{},
			gatewaybindings.NewResolver("local", repo),
			gatewayrouting.NewService(runtimepayload.NewBuiltinRegistry()),
			100,
			100,
			100,
			100,
		)
		svc.SetClock(func() time.Time { return now })
		runtime := &Runtime{
			ctx:          context.Background(),
			gateway:      svc,
			allowedUsers: map[int64]struct{}{1001: {}},
			logger:       logging.L("telegram"),
		}
		if err := runtime.handleText(ctx); err != nil {
			t.Fatalf("handleText: %v", err)
		}
		_ = logging.Sync()
	})

	line := findLogLine(out, "[telegram] telegram ingest accepted")
	if line == "" {
		t.Fatalf("expected telegram ingest accepted log in %q", out)
	}
	if !strings.Contains(line, "\tINFO\t") {
		t.Fatalf("expected INFO level log, got %q", line)
	}
	for _, part := range []string{
		`"event_id": "evt_accepted"`,
		`"session_key": "local:dm:telegram"`,
		`"session_id": "ses_accepted"`,
		`"conversation_id": "tg_chat_100"`,
		`"duplicate": false`,
		`"enqueued": true`,
		`"idempotency_key": "telegram:update:44"`,
		`"participant_id": "1001"`,
		`"update_id": 44`,
	} {
		if !strings.Contains(line, part) {
			t.Fatalf("missing %q in %q", part, line)
		}
	}
}

func TestHandleTextLogsIngestDuplicate(t *testing.T) {
	now := time.Now().UTC()
	ctx := tele.NewContext(nil, tele.Update{
		ID: 45,
		Message: &tele.Message{
			ID:   10,
			Text: "hello again",
			Chat: &tele.Chat{ID: 100, Type: tele.ChatPrivate},
			Sender: &tele.User{
				ID: 1001,
			},
		},
	})

	repo := &telegramRepoStub{
		result: gateway.PersistResult{
			EventID:     "evt_duplicate",
			SessionKey:  "local:dm:telegram",
			SessionID:   "ses_duplicate",
			ReceivedAt:  now,
			PayloadHash: "sha256:test",
			Duplicate:   true,
		},
	}

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		svc := gateway.NewService(
			"local",
			repo,
			telegramQueueStub{},
			gatewaybindings.NewResolver("local", repo),
			gatewayrouting.NewService(runtimepayload.NewBuiltinRegistry()),
			100,
			100,
			100,
			100,
		)
		svc.SetClock(func() time.Time { return now })
		runtime := &Runtime{
			ctx:          context.Background(),
			gateway:      svc,
			allowedUsers: map[int64]struct{}{1001: {}},
			logger:       logging.L("telegram"),
		}
		if err := runtime.handleText(ctx); err != nil {
			t.Fatalf("handleText: %v", err)
		}
		_ = logging.Sync()
	})

	line := findLogLine(out, "[telegram] telegram ingest duplicate")
	if line == "" {
		t.Fatalf("expected telegram ingest duplicate log in %q", out)
	}
	for _, part := range []string{
		`"event_id": "evt_duplicate"`,
		`"session_id": "ses_duplicate"`,
		`"duplicate": true`,
		`"enqueued": false`,
		`"update_id": 45`,
	} {
		if !strings.Contains(line, part) {
			t.Fatalf("missing %q in %q", part, line)
		}
	}
}

func TestHandleTextLogsAcceptedButNotEnqueued(t *testing.T) {
	now := time.Now().UTC()
	ctx := tele.NewContext(nil, tele.Update{
		ID: 46,
		Message: &tele.Message{
			ID:   11,
			Text: "hello deferred",
			Chat: &tele.Chat{ID: 100, Type: tele.ChatPrivate},
			Sender: &tele.User{
				ID: 1001,
			},
		},
	})

	repo := &telegramRepoStub{
		result: gateway.PersistResult{
			EventID:     "evt_deferred",
			SessionKey:  "local:dm:telegram",
			SessionID:   "ses_deferred",
			ReceivedAt:  now,
			PayloadHash: "sha256:test",
		},
	}

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		svc := gateway.NewService(
			"local",
			repo,
			telegramRejectQueueStub{},
			gatewaybindings.NewResolver("local", repo),
			gatewayrouting.NewService(runtimepayload.NewBuiltinRegistry()),
			100,
			100,
			100,
			100,
		)
		svc.SetClock(func() time.Time { return now })
		runtime := &Runtime{
			ctx:          context.Background(),
			gateway:      svc,
			allowedUsers: map[int64]struct{}{1001: {}},
			logger:       logging.L("telegram"),
		}
		if err := runtime.handleText(ctx); err != nil {
			t.Fatalf("handleText: %v", err)
		}
		_ = logging.Sync()
	})

	line := findLogLine(out, "[telegram] telegram ingest accepted but not enqueued")
	if line == "" {
		t.Fatalf("expected telegram ingest accepted but not enqueued log in %q", out)
	}
	if !strings.Contains(line, "\tWARN\t") {
		t.Fatalf("expected WARN level log, got %q", line)
	}
	for _, part := range []string{
		`"event_id": "evt_deferred"`,
		`"session_id": "ses_deferred"`,
		`"duplicate": false`,
		`"enqueued": false`,
		`"update_id": 46`,
	} {
		if !strings.Contains(line, part) {
			t.Fatalf("missing %q in %q", part, line)
		}
	}
}

func TestHandleTextLogsIngestRejected(t *testing.T) {
	ctx := tele.NewContext(nil, tele.Update{
		ID: 43,
		Message: &tele.Message{
			ID:   8,
			Text: "hello",
			Chat: &tele.Chat{ID: 100, Type: tele.ChatPrivate},
			Sender: &tele.User{
				ID: 1001,
			},
		},
	})

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		svc := gateway.NewService("local", nil, nil, nil, nil, 100, 100, 100, 100)
		runtime := &Runtime{
			ctx:          context.Background(),
			gateway:      svc,
			allowedUsers: map[int64]struct{}{1001: {}},
			logger:       logging.L("telegram"),
		}
		if err := runtime.handleText(ctx); err != nil {
			t.Fatalf("handleText: %v", err)
		}
		_ = logging.Sync()
	})

	line := ""
	for _, candidate := range strings.Split(out, "\n") {
		if strings.Contains(candidate, "[telegram] telegram ingest rejected") {
			line = candidate
			break
		}
	}
	if line == "" {
		t.Fatalf("expected telegram ingest rejected log in %q", out)
	}
	if !strings.Contains(line, "\tWARN\t") {
		t.Fatalf("expected WARN level log, got %q", line)
	}
	for _, part := range []string{
		`"error_code": "INTERNAL"`,
		`"status_code": 500`,
		`"update_id": 43`,
	} {
		if !strings.Contains(line, part) {
			t.Fatalf("missing %q in %q", part, line)
		}
	}
}

func TestHandleTextDelegatesNormalizedIngressToGatewaySeam(t *testing.T) {
	capture := &captureTelegramGateway{
		accepted: gateway.AcceptedIngest{
			Result: gateway.Result{
				EventID:    "evt_capture",
				SessionKey: "local:dm:telegram",
				SessionID:  "ses_capture",
				Enqueued:   true,
			},
		},
	}
	ctx := tele.NewContext(nil, tele.Update{
		ID: 47,
		Message: &tele.Message{
			ID:   12,
			Text: "hello seam",
			Chat: &tele.Chat{ID: 2001, Type: tele.ChatPrivate},
			Sender: &tele.User{
				ID: 1001,
			},
		},
	})

	runtime := &Runtime{
		ctx:          context.Background(),
		gateway:      capture,
		allowedUsers: map[int64]struct{}{1001: {}},
		logger:       logging.L("telegram-test"),
	}
	if err := runtime.handleText(ctx); err != nil {
		t.Fatalf("handleText: %v", err)
	}
	if capture.got.Source != "telegram" {
		t.Fatalf("expected telegram source, got %+v", capture.got)
	}
	if capture.got.IdempotencyKey != "telegram:update:47" {
		t.Fatalf("unexpected idempotency key: %+v", capture.got)
	}
	if capture.got.Payload.Text != "hello seam" {
		t.Fatalf("expected normalized text payload, got %+v", capture.got)
	}
	if capture.got.Conversation.ConversationID != "tg_chat_2001" {
		t.Fatalf("unexpected normalized conversation: %+v", capture.got.Conversation)
	}
}

func waitForHeartbeatCount(t *testing.T, recorder *fakeHeartbeatRecorder, want int) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if recorder.count() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected at least %d heartbeat calls, got %d", want, recorder.count())
}

func findLogLine(out string, pattern string) string {
	for _, candidate := range strings.Split(out, "\n") {
		if strings.Contains(candidate, pattern) {
			return candidate
		}
	}
	return ""
}

func newTelegramAPIServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = io.WriteString(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"bot","username":"test_bot"}}`)
		case strings.HasSuffix(r.URL.Path, "/deleteWebhook"):
			_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			time.Sleep(5 * time.Millisecond)
			_, _ = io.WriteString(w, `{"ok":true,"result":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
}
