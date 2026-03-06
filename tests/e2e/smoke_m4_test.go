package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestSmokeM4_PatchCommandRoundTrip(t *testing.T) {
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	app, err := api.NewApp(cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	app.Start()
	defer app.Stop()

	req := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "smoke_m4", ChannelType: "dm", ParticipantID: "u4"},
		IdempotencyKey: "cli:smoke_m4:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload: model.EventPayload{
			Type: "message",
			Text: "/patch workflows/smoke_m4.yaml",
		},
	}
	resp := ingest(t, app, req, 202)
	rec := pollEvent(t, app, resp.EventID)
	if rec.Status != model.EventStatusCommitted {
		t.Fatalf("expected committed status, got %+v", rec)
	}
	if !strings.Contains(rec.AssistantReply, "已收到") {
		t.Fatalf("expected local ADK echo reply, got %+v", rec)
	}
}
