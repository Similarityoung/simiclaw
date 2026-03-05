package api

import (
	"context"
	"testing"
	"time"

	adksession "google.golang.org/adk/session"

	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestNewAppInitializesADKRuntimeByDefault(t *testing.T) {
	cfg := config.Default()
	cfg.Workspace = t.TempDir()

	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	if app.ADKRuntime == nil {
		t.Fatalf("expected ADK runtime to be initialized")
	}
}

func TestIngestRoutesToADKSession(t *testing.T) {
	cfg := config.Default()
	cfg.Workspace = t.TempDir()

	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	if app.ADKRuntime == nil {
		t.Fatalf("expected ADK runtime to be initialized")
	}

	now := time.Now().UTC()
	req := model.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: "conv_adk",
			ChannelType:    "dm",
			ParticipantID:  "u_adk",
		},
		IdempotencyKey: "cli:conv_adk:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload: model.EventPayload{
			Type: "message",
			Text: "hello adk",
		},
	}

	resp, statusCode, apiErr := app.Gateway.Ingest(context.Background(), req)
	if apiErr != nil {
		t.Fatalf("ingest error: %v", apiErr)
	}
	if statusCode != 202 {
		t.Fatalf("expected 202 accepted, got %d", statusCode)
	}

	_, err = app.ADKRuntime.SessionService().Get(context.Background(), &adksession.GetRequest{
		AppName:   defaultGatewayADKAppName,
		UserID:    "u_adk",
		SessionID: resp.ActiveSessionID,
	})
	if err != nil {
		t.Fatalf("expected ADK session to be created, got error: %v", err)
	}
}
