package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/bus"
	"github.com/similarityyoung/simiclaw/pkg/config"
	runtime "github.com/similarityyoung/simiclaw/pkg/eventing"
	"github.com/similarityyoung/simiclaw/pkg/idempotency"
	"github.com/similarityyoung/simiclaw/pkg/model"
	store "github.com/similarityyoung/simiclaw/pkg/persistence"
)

type adkRouterStub struct {
	err            error
	calls          int
	lastReq        model.IngestRequest
	lastSessionKey string
	lastSessionID  string
}

func (s *adkRouterStub) RouteIngest(_ context.Context, req model.IngestRequest, sessionKey, sessionID string) error {
	s.calls++
	s.lastReq = req
	s.lastSessionKey = sessionKey
	s.lastSessionID = sessionID
	return s.err
}

func TestIngestRoutesToADKWithResolvedSession(t *testing.T) {
	router := &adkRouterStub{}
	svc, eventBus, idem, events := newGatewayServiceForIngestADKTests(t, router)
	req := testIngestRequest("conv_adk_ok", "u1", "cli:conv_adk_ok:1", "hello adk")

	resp, code, apiErr := svc.Ingest(context.Background(), req)
	if apiErr != nil {
		t.Fatalf("expected nil api error, got: %+v", apiErr)
	}
	if code != 202 {
		t.Fatalf("expected 202, got %d", code)
	}
	if router.calls != 1 {
		t.Fatalf("expected router call once, got %d", router.calls)
	}
	if router.lastReq.IdempotencyKey != req.IdempotencyKey {
		t.Fatalf("expected router to receive idempotency key %q, got %q", req.IdempotencyKey, router.lastReq.IdempotencyKey)
	}
	if router.lastSessionKey == "" || router.lastSessionID == "" {
		t.Fatalf("expected resolved session key and session id, got key=%q id=%q", router.lastSessionKey, router.lastSessionID)
	}
	if router.lastSessionKey != resp.SessionKey || router.lastSessionID != resp.ActiveSessionID {
		t.Fatalf("expected response session mapping to match router call")
	}

	row, ok := idem.LookupInbound(req.IdempotencyKey)
	if !ok {
		t.Fatalf("expected inbound idempotency row to be registered")
	}
	if row.ActiveSessionID != resp.ActiveSessionID {
		t.Fatalf("expected inbound row active_session_id %q, got %q", resp.ActiveSessionID, row.ActiveSessionID)
	}

	rec, ok := events.Get(resp.EventID)
	if !ok {
		t.Fatalf("expected event %q to be persisted", resp.EventID)
	}
	if rec.Status != model.EventStatusAccepted {
		t.Fatalf("expected event status accepted, got %s", rec.Status)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	evt, ok := eventBus.ConsumeInbound(ctx)
	if !ok {
		t.Fatalf("expected published inbound event")
	}
	if evt.ActiveSessionID != resp.ActiveSessionID || evt.SessionKey != resp.SessionKey {
		t.Fatalf("expected published event to carry resolved session mapping")
	}
}

func TestIngestReturnsInternalWhenADKRouterFailsAndRollsBackIdempotency(t *testing.T) {
	router := &adkRouterStub{err: errors.New("route failed")}
	svc, _, idem, _ := newGatewayServiceForIngestADKTests(t, router)
	req := testIngestRequest("conv_adk_fail", "u1", "cli:conv_adk_fail:1", "hello")

	_, _, apiErr := svc.Ingest(context.Background(), req)
	if apiErr == nil {
		t.Fatalf("expected api error when adk router fails")
	}
	if apiErr.StatusCode != 500 {
		t.Fatalf("expected 500 status code, got %d", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Message, "adk gateway route failed") {
		t.Fatalf("expected adk route failure message, got %q", apiErr.Message)
	}
	if router.calls != 1 {
		t.Fatalf("expected router call once, got %d", router.calls)
	}
	if _, ok := idem.LookupInbound(req.IdempotencyKey); ok {
		t.Fatalf("expected inbound idempotency row to be rolled back")
	}
}

func TestIngestReturnsInternalWhenADKRouterNotConfigured(t *testing.T) {
	svc, _, idem, _ := newGatewayServiceForIngestADKTests(t, nil)
	req := testIngestRequest("conv_adk_missing", "u1", "cli:conv_adk_missing:1", "hello")

	_, _, apiErr := svc.Ingest(context.Background(), req)
	if apiErr == nil {
		t.Fatalf("expected api error when adk router is nil")
	}
	if apiErr.StatusCode != 500 {
		t.Fatalf("expected 500 status code, got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "adk router is not configured" {
		t.Fatalf("expected adk router not configured message, got %q", apiErr.Message)
	}
	if _, ok := idem.LookupInbound(req.IdempotencyKey); ok {
		t.Fatalf("expected no inbound idempotency row when adk router is not configured")
	}
}

func newGatewayServiceForIngestADKTests(t *testing.T, router adkSessionRouter) (*Service, *bus.MessageBus, *idempotency.Store, *runtime.EventRepo) {
	t.Helper()

	workspace := t.TempDir()
	if err := store.InitWorkspace(workspace); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	cfg := config.Default()
	cfg.Workspace = workspace

	eventBus := bus.NewMessageBus(8)
	idem, err := idempotency.New(workspace)
	if err != nil {
		t.Fatalf("new idempotency store: %v", err)
	}
	sessions, err := store.NewSessionStore(workspace)
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}
	events, err := runtime.NewEventRepo(workspace)
	if err != nil {
		t.Fatalf("new event repo: %v", err)
	}

	return NewService(cfg, eventBus, idem, sessions, events, router), eventBus, idem, events
}

func testIngestRequest(conversationID, participantID, idempotencyKey, text string) model.IngestRequest {
	return model.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: conversationID,
			ChannelType:    "dm",
			ParticipantID:  participantID,
		},
		IdempotencyKey: idempotencyKey,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload: model.EventPayload{
			Type: "message",
			Text: text,
		},
	}
}
