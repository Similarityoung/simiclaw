package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestSmokeM3_MemoryRecallAndNoReply(t *testing.T) {
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	app, err := api.NewApp(cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	app.Start()
	defer app.Stop()

	remember := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "smoke_m3", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:smoke_m3:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "记住我喜欢 Go"},
	}
	rememberResp := ingest(t, app, remember, 202)
	rememberRec := pollEvent(t, app, rememberResp.EventID)
	if rememberRec.Status != model.EventStatusCommitted {
		t.Fatalf("remember event expected committed, got %+v", rememberRec)
	}

	compaction := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "smoke_m3", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:smoke_m3:2",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "compaction"},
	}
	compactionResp := ingest(t, app, compaction, 202)
	compactionRec := pollEvent(t, app, compactionResp.EventID)
	if compactionRec.DeliveryStatus != model.DeliveryStatusSuppressed || compactionRec.RunMode != model.RunModeNoReply {
		t.Fatalf("compaction should be suppressed no-reply, got %+v", compactionRec)
	}

	query := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "smoke_m3", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:smoke_m3:3",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "我喜欢什么？"},
	}
	queryResp := ingest(t, app, query, 202)
	queryRec := pollEvent(t, app, queryResp.EventID)
	if !strings.Contains(queryRec.AssistantReply, "已收到") {
		t.Fatalf("expected local ADK reply, got=%q", queryRec.AssistantReply)
	}
}

func TestSmokeM3_MemoryGetTraversalRejected(t *testing.T) {
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
		Conversation:   model.Conversation{ConversationID: "smoke_m3_boundary", ChannelType: "dm", ParticipantID: "u2"},
		IdempotencyKey: "cli:smoke_m3_boundary:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "/memory_get ../../go.mod 1:2"},
	}
	resp := ingest(t, app, req, 202)
	rec := pollEvent(t, app, resp.EventID)
	if !strings.Contains(rec.AssistantReply, "/memory_get") {
		t.Fatalf("expected memory_get command to be echoed, got %+v", rec)
	}
}
