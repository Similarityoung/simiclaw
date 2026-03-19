//go:build integration

package integration

import (
	"net/http"
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
