package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/config"
	runtime "github.com/similarityyoung/simiclaw/pkg/eventing"
	"github.com/similarityyoung/simiclaw/pkg/idempotency"
	"github.com/similarityyoung/simiclaw/pkg/model"
	store "github.com/similarityyoung/simiclaw/pkg/persistence"
)

type adkRouterStub struct {
	err            error
	runErr         error
	calls          int
	runCalls       int
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

func (s *adkRouterStub) RunIngest(_ context.Context, req model.IngestRequest, sessionKey, sessionID string) (ADKRunResult, error) {
	s.runCalls++
	s.lastReq = req
	s.lastSessionKey = sessionKey
	s.lastSessionID = sessionID
	if s.runErr != nil {
		return ADKRunResult{}, s.runErr
	}
	now := time.Now().UTC()
	runID := "run_test_1"
	trace := model.RunTrace{
		RunID:      runID,
		SessionKey: sessionKey,
		SessionID:  sessionID,
		RunMode:    model.RunModeNormal,
		Actions:    []model.Action{},
		StartedAt:  now,
		FinishedAt: now,
	}
	entries := []model.SessionEntry{{Type: "user", EntryID: "e_u", RunID: runID, Content: req.Payload.Text}, {Type: "assistant", EntryID: "e_a", RunID: runID, Content: "stub reply"}}
	return ADKRunResult{RunID: runID, RunMode: model.RunModeNormal, Entries: entries, Trace: trace, OutboundBody: "stub reply"}, nil
}

func TestIngestRoutesToADKWithResolvedSession(t *testing.T) {
	router := &adkRouterStub{}
	svc, idem, events := newGatewayServiceForIngestADKTests(t, router)
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
	if router.runCalls != 1 {
		t.Fatalf("expected run call once, got %d", router.runCalls)
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
	if rec.Status != model.EventStatusCommitted {
		t.Fatalf("expected event status committed, got %s", rec.Status)
	}
	if rec.AssistantReply != "stub reply" {
		t.Fatalf("expected assistant reply from run result, got %q", rec.AssistantReply)
	}
}

func TestIngestReturnsInternalWhenADKRouterFailsAndRollsBackIdempotency(t *testing.T) {
	router := &adkRouterStub{err: errors.New("route failed")}
	svc, idem, _ := newGatewayServiceForIngestADKTests(t, router)
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
	svc, idem, _ := newGatewayServiceForIngestADKTests(t, nil)
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

func newGatewayServiceForIngestADKTests(t *testing.T, router adkSessionRouter) (*Service, *idempotency.Store, *runtime.EventRepo) {
	t.Helper()

	workspace := t.TempDir()
	if err := store.InitWorkspace(workspace); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	cfg := config.Default()
	cfg.Workspace = workspace

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
	storeLoop := store.NewStoreLoop(workspace, sessions)
	storeLoop.Start()
	t.Cleanup(storeLoop.Stop)

	return NewService(cfg, idem, sessions, events, storeLoop, router), idem, events
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
