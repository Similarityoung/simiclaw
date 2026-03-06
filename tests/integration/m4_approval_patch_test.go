//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestM4PatchCommandRoundTrip(t *testing.T) {
	app := newTestApp(t, true, 32)
	req := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv_m4_patch", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv_m4_patch:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "/patch workflows/m4.yaml"},
	}
	resp, code := postIngest(t, app, req)
	if code != 202 {
		t.Fatalf("ingest expected 202, got %d", code)
	}
	rec := waitEvent(t, app, resp.EventID, 2*time.Second)
	if rec.Status != model.EventStatusCommitted {
		t.Fatalf("expected committed, got %+v", rec)
	}
	if !strings.Contains(rec.AssistantReply, "已收到") {
		t.Fatalf("expected local ADK reply, got %q", rec.AssistantReply)
	}
}
