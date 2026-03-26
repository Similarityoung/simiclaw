//go:build integration

package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestHTTPIngressUsesGatewaySessionHintBinding(t *testing.T) {
	app := newTestApp(t)
	conversation := model.Conversation{ConversationID: "http-gateway-binding", ChannelType: "dm", ParticipantID: "u1"}

	first := ingest(t, app, api.IngestRequest{
		Source:         "cli",
		Conversation:   conversation,
		DMScope:        "scope_http_a",
		IdempotencyKey: "cli:http-gateway-binding:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "first"},
	}, http.StatusAccepted)
	firstEvent := pollEvent(t, app, first.EventID)
	if firstEvent.SessionKey != first.SessionKey {
		t.Fatalf("expected first event session key to match ingest response, event=%+v resp=%+v", firstEvent, first)
	}

	second := ingest(t, app, api.IngestRequest{
		Source:         "cli",
		Conversation:   conversation,
		SessionKeyHint: first.SessionKey,
		IdempotencyKey: "cli:http-gateway-binding:2",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "second"},
	}, http.StatusAccepted)
	if second.SessionKey != first.SessionKey {
		t.Fatalf("expected session hint to keep binding session key, first=%+v second=%+v", first, second)
	}
	secondEvent := pollEvent(t, app, second.EventID)
	if secondEvent.SessionKey != first.SessionKey {
		t.Fatalf("expected persisted event to reuse hinted session key, event=%+v first=%+v", secondEvent, first)
	}
}

func TestHTTPChatStreamTerminalRecordMatchesQueryProjection(t *testing.T) {
	app := newTestApp(t)
	server := httptest.NewServer(app.Handler)
	defer server.Close()

	req := api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "http-stream-contract", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:http-stream-contract:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello contract"},
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := http.Post(server.URL+"/v1/chat:stream", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("chat stream request: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	accepted := readStreamEvent(t, reader)
	if accepted.Type != api.ChatStreamEventAccepted {
		t.Fatalf("expected accepted event, got %+v", accepted)
	}
	var done api.ChatStreamEvent
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		event := readStreamEvent(t, reader)
		if event.Type == api.ChatStreamEventDone {
			done = event
			break
		}
	}
	if done.EventRecord == nil {
		t.Fatalf("expected terminal event record, got %+v", done)
	}
	final := pollEvent(t, app, accepted.EventID)
	if final.EventID != done.EventRecord.EventID || final.Status != done.EventRecord.Status {
		t.Fatalf("expected stream terminal event to match query projection, stream=%+v query=%+v", done.EventRecord, final)
	}
}
