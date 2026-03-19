package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway/bindings"
	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
	"github.com/similarityyoung/simiclaw/internal/gateway/routing"
	"github.com/similarityyoung/simiclaw/internal/ingest/port"
	runtimepayload "github.com/similarityyoung/simiclaw/internal/runtime/payload"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type fakeRepo struct {
	result     port.PersistResult
	err        error
	markQueued int
	scope      string
	scopeOK    bool
	hint       bindings.SessionScopeRecord
	hintOK     bool
}

func (r *fakeRepo) PersistEvent(context.Context, string, string, port.PersistRequest, string, time.Time) (port.PersistResult, error) {
	return r.result, r.err
}

func (r *fakeRepo) MarkEventQueued(context.Context, string, time.Time) error {
	r.markQueued++
	return nil
}

func (r *fakeRepo) GetConversationDMScope(context.Context, string, model.Conversation) (string, bool, error) {
	return r.scope, r.scopeOK, nil
}

func (r *fakeRepo) GetScopeSession(context.Context, string) (bindings.SessionScopeRecord, bool, error) {
	return r.hint, r.hintOK, nil
}

type fakeQueue struct{}

func (fakeQueue) TryEnqueue(string) bool { return true }

func TestAcceptReturnsDuplicateAck(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepo{
		result: port.PersistResult{
			EventID:     "evt_dup",
			SessionKey:  "local:dm:u1",
			SessionID:   "ses_dup",
			ReceivedAt:  now,
			PayloadHash: "sha256:test",
			Duplicate:   true,
		},
	}
	svc := newGatewayServiceForTest(repo, fakeQueue{}, now)

	accepted, apiErr := svc.Accept(context.Background(), validGatewayIngress(now))
	if apiErr != nil {
		t.Fatalf("Accept apiErr=%+v", apiErr)
	}
	if accepted.StatusCode != 200 || accepted.Response.Status != ingestStatusDuplicate {
		t.Fatalf("expected duplicate ack response, got %+v", accepted)
	}
}

func TestAcceptMapsConflictError(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepo{err: port.ErrIdempotencyConflict}
	svc := newGatewayServiceForTest(repo, fakeQueue{}, now)

	_, apiErr := svc.Accept(context.Background(), validGatewayIngress(now))
	if apiErr == nil {
		t.Fatalf("expected api error")
	}
	if apiErr.StatusCode != 409 || apiErr.Code != model.ErrorCodeConflict {
		t.Fatalf("expected conflict mapping, got %+v", apiErr)
	}
}

func TestAcceptReturnsAcceptedResponse(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepo{
		result: port.PersistResult{
			EventID:     "evt_ok",
			SessionKey:  "local:dm:u1",
			SessionID:   "ses_ok",
			ReceivedAt:  now,
			PayloadHash: "sha256:test",
		},
	}
	svc := newGatewayServiceForTest(repo, fakeQueue{}, now)

	accepted, apiErr := svc.Accept(context.Background(), validGatewayIngress(now))
	if apiErr != nil {
		t.Fatalf("Accept apiErr=%+v", apiErr)
	}
	if accepted.StatusCode != 202 || accepted.Response.Status != ingestStatusAccepted {
		t.Fatalf("expected accepted response, got %+v", accepted)
	}
	if repo.markQueued != 1 {
		t.Fatalf("expected MarkEventQueued to run once, got %d", repo.markQueued)
	}
}

func TestAcceptUsesNewSessionPayloadOverride(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepo{
		result: port.PersistResult{
			EventID:     "evt_new",
			SessionKey:  "sk:new",
			SessionID:   "ses_new",
			ReceivedAt:  now,
			PayloadHash: "sha256:new",
		},
	}
	svc := newGatewayServiceForTest(repo, fakeQueue{}, now)

	accepted, apiErr := svc.Accept(context.Background(), gatewaymodel.NormalizedIngress{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv:1",
		Timestamp:      now,
		Payload:        model.EventPayload{Type: "message", Text: "/new"},
	})
	if apiErr != nil {
		t.Fatalf("Accept apiErr=%+v", apiErr)
	}
	if accepted.StatusCode != 202 {
		t.Fatalf("expected accepted response, got %+v", accepted)
	}
}

func TestAPIErrorErrorReturnsMessage(t *testing.T) {
	err := (&APIError{Message: "boom"}).Error()
	if err != "boom" {
		t.Fatalf("expected message, got %q", err)
	}
}

func newGatewayServiceForTest(repo *fakeRepo, queue Enqueuer, now time.Time) *Service {
	svc := NewService(
		"local",
		repo,
		queue,
		bindings.NewResolver("local", repo),
		routing.NewService(runtimepayload.NewBuiltinRegistry()),
		100,
		100,
		100,
		100,
	)
	svc.SetClock(func() time.Time { return now })
	return svc
}

func validGatewayIngress(now time.Time) gatewaymodel.NormalizedIngress {
	return gatewaymodel.NormalizedIngress{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv:1",
		Timestamp:      now,
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}
}
