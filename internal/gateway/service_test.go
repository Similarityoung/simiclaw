package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/ingest"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type fakeRepo struct {
	result     ingest.PersistResult
	err        error
	markQueued int
}

func (r *fakeRepo) PersistEvent(context.Context, string, string, ingest.PersistRequest, string, time.Time) (ingest.PersistResult, error) {
	return r.result, r.err
}

func (r *fakeRepo) MarkEventQueued(context.Context, string, time.Time) error {
	r.markQueued++
	return nil
}

type fakeQueue struct{}

func (fakeQueue) TryEnqueue(string) bool { return true }

type fakeResolver struct{}

func (fakeResolver) Resolve(_ context.Context, req api.IngestRequest) (api.IngestRequest, string, *ingest.Error) {
	if req.DMScope == "" {
		req.DMScope = "default"
	}
	return req, req.DMScope, nil
}

func TestAcceptReturnsDuplicateAck(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepo{
		result: ingest.PersistResult{
			EventID:     "evt_dup",
			SessionKey:  "local:dm:u1",
			SessionID:   "ses_dup",
			ReceivedAt:  now,
			PayloadHash: "sha256:test",
			Duplicate:   true,
		},
	}
	svc := NewService(ingest.NewService("local", repo, fakeQueue{}, fakeResolver{}, 100, 100, 100, 100))

	accepted, apiErr := svc.Accept(context.Background(), validGatewayRequest(now))
	if apiErr != nil {
		t.Fatalf("Accept apiErr=%+v", apiErr)
	}
	if accepted.StatusCode != 200 || accepted.Response.Status != ingestStatusDuplicate {
		t.Fatalf("expected duplicate ack response, got %+v", accepted)
	}
}

func TestAcceptMapsConflictError(t *testing.T) {
	repo := &fakeRepo{err: ingest.ErrIdempotencyConflict}
	svc := NewService(ingest.NewService("local", repo, fakeQueue{}, fakeResolver{}, 100, 100, 100, 100))

	_, apiErr := svc.Accept(context.Background(), validGatewayRequest(time.Now().UTC()))
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
		result: ingest.PersistResult{
			EventID:     "evt_ok",
			SessionKey:  "local:dm:u1",
			SessionID:   "ses_ok",
			ReceivedAt:  now,
			PayloadHash: "sha256:test",
		},
	}
	svc := NewService(ingest.NewService("local", repo, fakeQueue{}, fakeResolver{}, 100, 100, 100, 100))

	accepted, apiErr := svc.Accept(context.Background(), validGatewayRequest(now))
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

func TestIngestWrapsAccept(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepo{
		result: ingest.PersistResult{
			EventID:     "evt_ok",
			SessionKey:  "local:dm:u1",
			SessionID:   "ses_ok",
			ReceivedAt:  now,
			PayloadHash: "sha256:test",
		},
	}
	svc := NewService(ingest.NewService("local", repo, fakeQueue{}, fakeResolver{}, 100, 100, 100, 100))

	resp, status, apiErr := svc.Ingest(context.Background(), validGatewayRequest(now))
	if apiErr != nil {
		t.Fatalf("Ingest apiErr=%+v", apiErr)
	}
	if status != 202 || resp.EventID != "evt_ok" {
		t.Fatalf("unexpected ingest response status=%d resp=%+v", status, resp)
	}
}

func TestAPIErrorErrorReturnsMessage(t *testing.T) {
	err := (&APIError{Message: "boom"}).Error()
	if err != "boom" {
		t.Fatalf("expected message, got %q", err)
	}
}

func TestMapIngestErrorStatusMappings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    *ingest.Error
		status int
	}{
		{name: "invalid", err: &ingest.Error{Code: model.ErrorCodeInvalidArgument, Message: "bad"}, status: 400},
		{name: "rate", err: &ingest.Error{Code: model.ErrorCodeRateLimited, Message: "slow"}, status: 429},
		{name: "internal", err: &ingest.Error{Code: model.ErrorCodeInternal, Message: "boom"}, status: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			apiErr := mapIngestError(tc.err)
			if apiErr == nil || apiErr.StatusCode != tc.status {
				t.Fatalf("unexpected mapping: %+v", apiErr)
			}
		})
	}
}

func TestIngestReturnsAPIError(t *testing.T) {
	repo := &fakeRepo{err: ingest.ErrIdempotencyConflict}
	svc := NewService(ingest.NewService("local", repo, fakeQueue{}, fakeResolver{}, 100, 100, 100, 100))

	_, _, apiErr := svc.Ingest(context.Background(), validGatewayRequest(time.Now().UTC()))
	if apiErr == nil || apiErr.StatusCode != 409 {
		t.Fatalf("expected conflict api error, got %+v", apiErr)
	}
}

func validGatewayRequest(now time.Time) api.IngestRequest {
	return api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}
}
