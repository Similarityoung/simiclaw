package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestIngestRequestJSONShape(t *testing.T) {
	req := IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: "conv-1",
			ThreadID:       "th-1",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		DMScope:        "scope-a",
		SessionKeyHint: "ses-hint",
		IdempotencyKey: "cli:conv-1:1",
		Timestamp:      "2026-03-10T12:00:00Z",
		Payload: model.EventPayload{
			Type: "message",
			Text: "hello",
			Extra: map[string]string{
				"a": "b",
			},
		},
	}

	got, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal ingest request: %v", err)
	}
	want := `{"source":"cli","conversation":{"conversation_id":"conv-1","thread_id":"th-1","channel_type":"dm","participant_id":"u1"},"dm_scope":"scope-a","session_key":"ses-hint","idempotency_key":"cli:conv-1:1","timestamp":"2026-03-10T12:00:00Z","payload":{"type":"message","text":"hello","extra":{"a":"b"}}}`
	if string(got) != want {
		t.Fatalf("unexpected ingest request json:\nwant: %s\ngot:  %s", want, string(got))
	}
}

func TestIngestResponseJSONShape(t *testing.T) {
	resp := IngestResponse{
		EventID:         "evt-1",
		SessionKey:      "local:dm:u1",
		ActiveSessionID: "ses-1",
		ReceivedAt:      "2026-03-10T12:00:00Z",
		PayloadHash:     "sha256:abc",
		Status:          "accepted",
		StatusURL:       "/v1/events/evt-1",
		Error: &model.ErrorBlock{
			Code:    model.ErrorCodeConflict,
			Message: "conflict",
		},
	}

	got, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal ingest response: %v", err)
	}
	want := `{"event_id":"evt-1","session_key":"local:dm:u1","active_session_id":"ses-1","received_at":"2026-03-10T12:00:00Z","payload_hash":"sha256:abc","status":"accepted","status_url":"/v1/events/evt-1","error":{"code":"CONFLICT","message":"conflict"}}`
	if string(got) != want {
		t.Fatalf("unexpected ingest response json:\nwant: %s\ngot:  %s", want, string(got))
	}
}

func TestChatStreamWireCompatibility(t *testing.T) {
	if ChatStreamProtocolVersion != "2026-03-07.sse.v1" {
		t.Fatalf("unexpected protocol version: %s", ChatStreamProtocolVersion)
	}
	if ChatStreamEventAccepted != "accepted" || ChatStreamEventDone != "done" || ChatStreamEventError != "error" {
		t.Fatalf("unexpected event type constants")
	}

	event := ChatStreamEvent{
		Type:                  ChatStreamEventAccepted,
		EventID:               "evt-1",
		Sequence:              1,
		At:                    time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		StreamProtocolVersion: ChatStreamProtocolVersion,
		IngestResponse: &IngestResponse{
			EventID:         "evt-1",
			SessionKey:      "local:dm:u1",
			ActiveSessionID: "ses-1",
			ReceivedAt:      "2026-03-10T12:00:00Z",
			PayloadHash:     "sha256:abc",
			Status:          "accepted",
			StatusURL:       "/v1/events/evt-1",
		},
	}

	got, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal chat stream event: %v", err)
	}
	want := `{"type":"accepted","event_id":"evt-1","sequence":1,"at":"2026-03-10T12:00:00Z","stream_protocol_version":"2026-03-07.sse.v1","ingest_response":{"event_id":"evt-1","session_key":"local:dm:u1","active_session_id":"ses-1","received_at":"2026-03-10T12:00:00Z","payload_hash":"sha256:abc","status":"accepted","status_url":"/v1/events/evt-1"}}`
	if string(got) != want {
		t.Fatalf("unexpected chat stream event json:\nwant: %s\ngot:  %s", want, string(got))
	}
}
