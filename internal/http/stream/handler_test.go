package stream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
	"github.com/similarityyoung/simiclaw/internal/streaming"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type fakeGateway struct {
	accepted gateway.AcceptedIngest
	apiErr   *gateway.APIError
}

func (f fakeGateway) Accept(context.Context, gatewaymodel.NormalizedIngress) (gateway.AcceptedIngest, *gateway.APIError) {
	return f.accepted, f.apiErr
}

type fakeStreamQuery struct {
	record querymodel.EventRecord
	ok     bool
	err    error
}

func (f fakeStreamQuery) GetEvent(context.Context, string) (querymodel.EventRecord, bool, error) {
	return f.record, f.ok, f.err
}

func TestHandleChatStreamReplaysTerminalEventFromQuery(t *testing.T) {
	now := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	handler := NewHandlers(
		fakeGateway{
			accepted: gateway.AcceptedIngest{
				Response: api.IngestResponse{EventID: "evt_replay", Status: "accepted"},
				Result: gateway.Result{
					EventID:    "evt_replay",
					SessionKey: "local:dm:u1",
					SessionID:  "ses_1",
				},
				StatusCode: http.StatusAccepted,
			},
		},
		fakeStreamQuery{
			record: querymodel.EventRecord{
				EventID:        "evt_replay",
				Status:         model.EventStatusProcessed,
				AssistantReply: "done",
				UpdatedAt:      now,
			},
			ok: true,
		},
		streaming.NewHub(),
	)

	body, err := json.Marshal(api.IngestRequest{
		Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
		IdempotencyKey: "cli:conv:1",
		Timestamp:      now.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat:stream", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleChatStream(rec, req)
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected SSE content type, got %q", got)
	}

	reader := bufio.NewReader(bytes.NewReader(rec.Body.Bytes()))
	accepted := readStreamEventForTest(t, reader)
	if accepted.Type != api.ChatStreamEventAccepted || accepted.EventID != "evt_replay" {
		t.Fatalf("unexpected accepted event: %+v", accepted)
	}
	done := readStreamEventForTest(t, reader)
	if done.Type != api.ChatStreamEventDone {
		t.Fatalf("expected done event, got %+v", done)
	}
	if done.EventRecord == nil || done.EventRecord.AssistantReply != "done" {
		t.Fatalf("unexpected terminal payload: %+v", done)
	}
}

func readStreamEventForTest(t *testing.T, reader *bufio.Reader) api.ChatStreamEvent {
	t.Helper()
	eventType, data, err := readSSEEvent(reader)
	if err != nil {
		t.Fatalf("read SSE event: %v", err)
	}
	var event api.ChatStreamEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("decode SSE payload: %v", err)
	}
	if string(event.Type) != eventType {
		t.Fatalf("event type mismatch: header=%s payload=%s", eventType, event.Type)
	}
	return event
}
