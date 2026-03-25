package telegram

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/gateway"
	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	tele "gopkg.in/telebot.v4"
)

const runtimeHeartbeatInterval = 10 * time.Second

type runtimeTestHooks struct {
	mu     sync.RWMutex
	apiURL string
	client *http.Client
}

var testHooks runtimeTestHooks

type HeartbeatRecorder interface {
	BeatHeartbeat(ctx context.Context, name string, at time.Time) error
}

type Gateway interface {
	Accept(ctx context.Context, in gatewaymodel.NormalizedIngress) (gateway.AcceptedIngest, *gateway.APIError)
}

func SetRuntimeTestHooksForTesting(apiURL string, client *http.Client) func() {
	testHooks.mu.Lock()
	prevAPIURL := testHooks.apiURL
	prevClient := testHooks.client
	testHooks.apiURL = apiURL
	testHooks.client = client
	testHooks.mu.Unlock()
	return func() {
		testHooks.mu.Lock()
		testHooks.apiURL = prevAPIURL
		testHooks.client = prevClient
		testHooks.mu.Unlock()
	}
}

type Runtime struct {
	cfg               config.TelegramChannelConfig
	heartbeat         HeartbeatRecorder
	heartbeatInterval time.Duration
	gateway           Gateway
	allowedUsers      map[int64]struct{}
	logger            *logging.Logger

	mu      sync.Mutex
	bot     *tele.Bot
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

func NewRuntime(cfg config.TelegramChannelConfig, heartbeat HeartbeatRecorder, gatewayService Gateway) (*Runtime, error) {
	return &Runtime{
		cfg:               cfg,
		heartbeat:         heartbeat,
		heartbeatInterval: runtimeHeartbeatInterval,
		gateway:           gatewayService,
		allowedUsers:      newAllowedUsers(cfg.AllowedUserIDs),
		logger:            logging.L("telegram"),
	}, nil
}

func (r *Runtime) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return nil
	}
	r.logger.Info("telegram runtime starting")
	ctx, cancel := context.WithCancel(context.Background())
	bot, err := tele.NewBot(tele.Settings{
		Token:       r.cfg.Token,
		URL:         r.currentAPIURL(),
		Poller:      &tele.LongPoller{Timeout: r.cfg.LongPollTimeout.Duration, AllowedUpdates: []string{"message", "callback_query"}},
		Client:      r.currentHTTPClient(),
		Synchronous: true,
		OnError: func(err error, c tele.Context) {
			if c != nil {
				r.logger.Warn("telegram handler error", logging.Error(err), logging.Int("update_id", c.Update().ID))
				return
			}
			r.logger.Warn("telegram poller error", logging.Error(err))
		},
	})
	if err != nil {
		cancel()
		r.logger.Error("telegram bot init failed", logging.Error(err))
		return err
	}
	if err := bot.RemoveWebhook(false); err != nil {
		cancel()
		r.logger.Error("telegram webhook cleanup failed", logging.Error(err))
		return err
	}
	bot.Handle(tele.OnText, func(c tele.Context) error {
		return r.handleText(c)
	})
	bot.Handle(tele.OnCallback, func(c tele.Context) error {
		r.logger.Info("telegram update ignored", logging.String("reason", "callback_unsupported"), logging.Int("update_id", c.Update().ID))
		return nil
	})

	r.bot = bot
	r.ctx = ctx
	r.cancel = cancel
	r.started = true
	r.wg.Add(2)
	go r.runBot(bot)
	go r.runHeartbeat(ctx)
	r.logger.Info("telegram runtime started")
	return nil
}

func (r *Runtime) Stop() {
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		return
	}
	bot := r.bot
	cancel := r.cancel
	r.started = false
	r.bot = nil
	r.cancel = nil
	r.mu.Unlock()

	r.logger.Info("telegram runtime stopping")
	if cancel != nil {
		cancel()
	}
	if bot != nil {
		bot.Stop()
	}
	r.wg.Wait()
	r.logger.Info("telegram runtime stopped")
}

func (r *Runtime) SendTextMessage(ctx context.Context, chatID int64, text string) error {
	r.mu.Lock()
	bot := r.bot
	r.mu.Unlock()
	if bot == nil {
		return fmt.Errorf("telegram runtime is not started")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	_, err := bot.Send(&tele.Chat{ID: chatID}, text)
	return err
}

func (r *Runtime) handleText(c tele.Context) error {
	msg := c.Message()
	allowed, reason := isAllowedPrivateTextMessage(r.botUserID(), r.allowedUsers, msg)
	if !allowed {
		r.logger.Info("telegram update ignored", logging.String("reason", reason), logging.Int("update_id", c.Update().ID))
		return nil
	}
	receivedAt := time.Now().UTC()
	req, err := NormalizeTextUpdate(c.Update(), receivedAt)
	if err != nil {
		r.logger.Warn("telegram normalize failed", logging.Error(err), logging.Int("update_id", c.Update().ID))
		return nil
	}
	accepted, apiErr := r.gateway.Accept(r.ctx, req)
	if apiErr != nil {
		r.logger.Warn("telegram ingest rejected",
			logging.Int("update_id", c.Update().ID),
			logging.Int("status_code", apiErr.StatusCode),
			logging.String("error_code", apiErr.Code),
			logging.String("message", apiErr.Message),
		)
		return nil
	}
	fields := []logging.Field{
		logging.Int("update_id", c.Update().ID),
		logging.String("event_id", accepted.Result.EventID),
		logging.String("session_key", accepted.Result.SessionKey),
		logging.String("session_id", accepted.Result.SessionID),
		logging.String("conversation_id", req.Conversation.ConversationID),
		logging.String("participant_id", req.Conversation.ParticipantID),
		logging.String("idempotency_key", req.IdempotencyKey),
		logging.Bool("duplicate", accepted.Result.Duplicate),
		logging.Bool("enqueued", accepted.Result.Enqueued),
	}
	switch {
	case accepted.Result.Duplicate:
		r.logger.Info("telegram ingest duplicate", fields...)
	case accepted.Result.Enqueued:
		r.logger.Info("telegram ingest accepted", fields...)
	default:
		r.logger.Warn("telegram ingest accepted but not enqueued", fields...)
	}
	return nil
}

func (r *Runtime) runBot(bot *tele.Bot) {
	defer r.wg.Done()
	bot.Start()
}

func (r *Runtime) runHeartbeat(ctx context.Context) {
	defer r.wg.Done()
	if r.heartbeat == nil {
		return
	}
	beat := func() {
		if err := r.heartbeat.BeatHeartbeat(ctx, "telegram_polling", time.Now().UTC()); err != nil {
			r.logger.Warn("telegram heartbeat failed", logging.Error(err))
		}
	}
	beat()
	interval := r.heartbeatInterval
	if interval <= 0 {
		interval = runtimeHeartbeatInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			beat()
		}
	}
}

func (r *Runtime) botUserID() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.bot == nil || r.bot.Me == nil {
		return 0
	}
	return r.bot.Me.ID
}

func (r *Runtime) currentAPIURL() string {
	testHooks.mu.RLock()
	defer testHooks.mu.RUnlock()
	if testHooks.apiURL != "" {
		return testHooks.apiURL
	}
	return tele.DefaultApiURL
}

func (r *Runtime) currentHTTPClient() *http.Client {
	testHooks.mu.RLock()
	defer testHooks.mu.RUnlock()
	if testHooks.client != nil {
		return testHooks.client
	}
	timeout := r.cfg.LongPollTimeout.Duration + 10*time.Second
	if timeout <= 0 {
		timeout = 40 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func newAllowedUsers(ids []int64) map[int64]struct{} {
	out := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}
