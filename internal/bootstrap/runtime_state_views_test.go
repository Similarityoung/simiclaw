package bootstrap

import (
	"context"
	"testing"
	"time"

	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestRuntimeEventRecordViewMapsQueryRecord(t *testing.T) {
	now := time.Date(2026, 3, 25, 9, 0, 0, 0, time.UTC)
	view := runtimeEventRecordView{
		query: stubEventRecordQuery{
			record: querymodel.EventRecord{
				EventID:           "evt_1",
				Status:            model.EventStatusProcessed,
				OutboxStatus:      model.OutboxStatusSent,
				SessionKey:        "session:1",
				SessionID:         "ses_1",
				RunID:             "run_1",
				RunMode:           model.RunModeNormal,
				AssistantReply:    "done",
				OutboxID:          "out_1",
				ProcessingLease:   now.Format(time.RFC3339Nano),
				ReceivedAt:        now.Add(-time.Minute),
				CreatedAt:         now.Add(-2 * time.Minute),
				UpdatedAt:         now,
				PayloadHash:       "sha256:1",
				Provider:          "openai",
				Model:             "gpt",
				ProviderRequestID: "req_1",
				Error:             &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: "x"},
			},
			ok: true,
		},
	}

	rec, ok, err := view.GetEventRecord(context.Background(), "evt_1")
	if err != nil {
		t.Fatalf("GetEventRecord: %v", err)
	}
	if !ok {
		t.Fatal("expected event record")
	}
	if rec.EventID != "evt_1" || rec.SessionKey != "session:1" || rec.OutboxStatus != model.OutboxStatusSent {
		t.Fatalf("unexpected runtime event record: %+v", rec)
	}
	if rec.Error == nil || rec.Error.Message != "x" {
		t.Fatalf("expected mapped error block, got %+v", rec.Error)
	}
}

func TestGatewaySessionLookupMapsSessionProjection(t *testing.T) {
	lookup := gatewaySessionLookup{
		scopes: stubConversationScopeQuery{scope: "scope_saved", ok: true},
		sessions: stubSessionRecordQuery{
			record: querymodel.SessionRecord{
				SessionKey:      "session:1",
				ActiveSessionID: "ses_1",
				ConversationID:  "conv_1",
				ChannelType:     "dm",
				ParticipantID:   "u1",
				DMScope:         "scope_saved",
			},
			ok: true,
		},
	}

	scope, ok, err := lookup.GetConversationDMScope(context.Background(), "local", model.Conversation{ConversationID: "conv_1"})
	if err != nil {
		t.Fatalf("GetConversationDMScope: %v", err)
	}
	if !ok || scope != "scope_saved" {
		t.Fatalf("unexpected conversation scope: %q ok=%v", scope, ok)
	}

	rec, ok, err := lookup.GetScopeSession(context.Background(), "session:1")
	if err != nil {
		t.Fatalf("GetScopeSession: %v", err)
	}
	if !ok {
		t.Fatal("expected scope session")
	}
	if rec.SessionID != "ses_1" || rec.DMScope != "scope_saved" || rec.ConversationID != "conv_1" {
		t.Fatalf("unexpected session scope record: %+v", rec)
	}
}

type stubEventRecordQuery struct {
	record querymodel.EventRecord
	ok     bool
	err    error
}

func (s stubEventRecordQuery) GetEventRecord(context.Context, string) (querymodel.EventRecord, bool, error) {
	return s.record, s.ok, s.err
}

type stubConversationScopeQuery struct {
	scope string
	ok    bool
	err   error
}

func (s stubConversationScopeQuery) GetConversationDMScope(context.Context, string, model.Conversation) (string, bool, error) {
	return s.scope, s.ok, s.err
}

type stubSessionRecordQuery struct {
	record querymodel.SessionRecord
	ok     bool
	err    error
}

func (s stubSessionRecordQuery) GetSessionRecord(context.Context, string) (querymodel.SessionRecord, bool, error) {
	return s.record, s.ok, s.err
}
