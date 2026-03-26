package query

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type fakeQuery struct {
	getEvent    func(context.Context, string) (querymodel.EventRecord, bool, error)
	listEvents  func(context.Context, querymodel.EventFilter) (querymodel.EventPage, error)
	lookupEvent func(context.Context, string) (querymodel.LookupEvent, bool, error)
}

func (f fakeQuery) GetEvent(ctx context.Context, eventID string) (querymodel.EventRecord, bool, error) {
	if f.getEvent != nil {
		return f.getEvent(ctx, eventID)
	}
	return querymodel.EventRecord{}, false, nil
}

func (f fakeQuery) LookupEvent(ctx context.Context, idempotencyKey string) (querymodel.LookupEvent, bool, error) {
	if f.lookupEvent != nil {
		return f.lookupEvent(ctx, idempotencyKey)
	}
	return querymodel.LookupEvent{}, false, nil
}

func (f fakeQuery) ListEvents(ctx context.Context, filter querymodel.EventFilter) (querymodel.EventPage, error) {
	if f.listEvents != nil {
		return f.listEvents(ctx, filter)
	}
	return querymodel.EventPage{}, nil
}

func (f fakeQuery) ListRuns(context.Context, querymodel.RunFilter) (querymodel.RunPage, error) {
	return querymodel.RunPage{}, nil
}

func (f fakeQuery) GetRun(context.Context, string) (querymodel.RunTrace, bool, error) {
	return querymodel.RunTrace{}, false, nil
}

func (f fakeQuery) ListSessions(context.Context, querymodel.SessionFilter) (querymodel.SessionPage, error) {
	return querymodel.SessionPage{}, nil
}

func (f fakeQuery) GetSession(context.Context, string) (querymodel.SessionRecord, bool, error) {
	return querymodel.SessionRecord{}, false, nil
}

func (f fakeQuery) ListSessionHistory(context.Context, querymodel.SessionHistoryFilter) (querymodel.MessagePage, error) {
	return querymodel.MessagePage{}, nil
}

func TestHandleGetEventDelegatesToQuerySeam(t *testing.T) {
	var gotEventID string
	h := NewHandlers(fakeQuery{
		getEvent: func(_ context.Context, eventID string) (querymodel.EventRecord, bool, error) {
			gotEventID = eventID
			return querymodel.EventRecord{
				EventID:        eventID,
				Status:         model.EventStatusProcessed,
				SessionKey:     "local:dm:u1",
				SessionID:      "ses_1",
				AssistantReply: "done",
				PayloadHash:    "hash_event",
			}, true, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/events/evt_123", nil)
	req.SetPathValue("event_id", "evt_123")
	rec := httptest.NewRecorder()
	h.HandleGetEvent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if gotEventID != "evt_123" {
		t.Fatalf("expected query seam to receive evt_123, got %q", gotEventID)
	}
	var body api.EventRecord
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.EventID != "evt_123" || body.AssistantReply != "done" {
		t.Fatalf("unexpected event body: %+v", body)
	}
}

func TestHandleListEventsDecodesCreatedAtCursor(t *testing.T) {
	wantTime := time.Date(2026, 3, 19, 8, 30, 0, 0, time.UTC)
	var gotFilter querymodel.EventFilter
	h := NewHandlers(fakeQuery{
		listEvents: func(_ context.Context, filter querymodel.EventFilter) (querymodel.EventPage, error) {
			gotFilter = filter
			return querymodel.EventPage{}, nil
		},
	})

	cursor := encodeCursor(eventCursor{
		V:             1,
		LastCreatedAt: wantTime.Format(time.RFC3339Nano),
		LastEventID:   "evt_1",
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/events?limit=2&cursor="+cursor, nil)
	rec := httptest.NewRecorder()
	h.HandleListEvents(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if gotFilter.Limit != 2 {
		t.Fatalf("expected limit 2, got %+v", gotFilter)
	}
	if gotFilter.Cursor == nil {
		t.Fatalf("expected decoded cursor, got %+v", gotFilter)
	}
	if !gotFilter.Cursor.CreatedAt.Equal(wantTime) || gotFilter.Cursor.EventID != "evt_1" {
		t.Fatalf("expected created_at cursor anchor, got %+v", gotFilter.Cursor)
	}
}

func TestHandleLookupEventRequiresIdempotencyKey(t *testing.T) {
	h := NewHandlers(fakeQuery{})
	req := httptest.NewRequest(http.MethodGet, "/v1/events:lookup", nil)
	rec := httptest.NewRecorder()
	h.HandleLookupEvent(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body api.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != model.ErrorCodeInvalidArgument {
		t.Fatalf("unexpected error body: %+v", body.Error)
	}
}
