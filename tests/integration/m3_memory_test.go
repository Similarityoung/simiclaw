//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestM3MemoryRecallAfterCompaction(t *testing.T) {
	app := newTestApp(t, true, 32)

	rememberReq := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv_m3_recall", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv_m3_recall:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "记住我喜欢 Go"},
	}
	rememberResp, code := postIngest(t, app, rememberReq)
	if code != 202 {
		t.Fatalf("remember ingest expected 202, got %d", code)
	}
	rememberRec := waitEvent(t, app, rememberResp.EventID, 2*time.Second)
	if rememberRec.Status != model.EventStatusCommitted {
		t.Fatalf("remember event expected committed, got %s", rememberRec.Status)
	}

	compactionReq := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv_m3_recall", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv_m3_recall:2",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "compaction"},
	}
	compactionResp, code := postIngest(t, app, compactionReq)
	if code != 202 {
		t.Fatalf("compaction ingest expected 202, got %d", code)
	}
	compactionRec := waitEvent(t, app, compactionResp.EventID, 2*time.Second)
	if compactionRec.RunMode != model.RunModeNoReply || compactionRec.DeliveryStatus != model.DeliveryStatusSuppressed {
		t.Fatalf("compaction should be NO_REPLY+suppress, got %+v", compactionRec)
	}

	queryReq := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv_m3_recall", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv_m3_recall:3",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "我喜欢什么？"},
	}
	queryResp, code := postIngest(t, app, queryReq)
	if code != 202 {
		t.Fatalf("query ingest expected 202, got %d", code)
	}
	queryRec := waitEvent(t, app, queryResp.EventID, 2*time.Second)
	if queryRec.Status != model.EventStatusCommitted {
		t.Fatalf("query event expected committed, got %s", queryRec.Status)
	}
	if !strings.Contains(queryRec.AssistantReply, "已收到") {
		t.Fatalf("assistant reply should come from local ADK model, got=%q", queryRec.AssistantReply)
	}
	body, status := doRequest(t, app, http.MethodGet, "/v1/runs/"+queryRec.RunID+"/trace?view=full", nil)
	if status != 200 {
		t.Fatalf("trace expected 200, got %d body=%s", status, string(body))
	}
}

func TestM3MemoryGetTraversalRejectedAndRedactedTrace(t *testing.T) {
	app := newTestApp(t, true, 32)

	req := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv_m3_boundary", ChannelType: "dm", ParticipantID: "u2"},
		IdempotencyKey: "cli:conv_m3_boundary:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "/memory_get ../../go.mod 1:2"},
	}
	resp, code := postIngest(t, app, req)
	if code != 202 {
		t.Fatalf("ingest expected 202, got %d", code)
	}
	rec := waitEvent(t, app, resp.EventID, 2*time.Second)
	if !strings.Contains(rec.AssistantReply, "/memory_get") {
		t.Fatalf("assistant reply should echo memory_get command, got=%q", rec.AssistantReply)
	}

	body, status := doRequest(t, app, http.MethodGet, "/v1/runs/"+rec.RunID+"/trace?view=full", nil)
	if status != 200 {
		t.Fatalf("trace expected 200, got %d body=%s", status, string(body))
	}
	var trace struct {
		ToolExecutions []struct {
			Name  string `json:"name"`
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		} `json:"tool_executions"`
	}
	if err := json.Unmarshal(body, &trace); err != nil {
		t.Fatalf("decode trace: %v", err)
	}
	if len(trace.ToolExecutions) != 0 {
		t.Fatalf("local ADK model path should not emit tool executions, got %+v", trace.ToolExecutions)
	}

	redactedBody, status := doRequest(t, app, http.MethodGet, "/v1/runs/"+rec.RunID+"/trace?view=full&redact=true", nil)
	if status != 200 {
		t.Fatalf("redacted trace expected 200, got %d body=%s", status, string(redactedBody))
	}
	if strings.Contains(string(redactedBody), "../../go.mod") {
		t.Fatalf("redacted trace should hide raw args, got=%s", string(redactedBody))
	}
}

func TestM3CronFireIsNoReply(t *testing.T) {
	app := newTestApp(t, true, 32)

	req := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv_m3_cron", ChannelType: "dm", ParticipantID: "u3"},
		IdempotencyKey: "cli:conv_m3_cron:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "cron_fire"},
	}
	resp, code := postIngest(t, app, req)
	if code != 202 {
		t.Fatalf("ingest expected 202, got %d", code)
	}
	rec := waitEvent(t, app, resp.EventID, 2*time.Second)
	if rec.RunMode != model.RunModeNoReply || rec.DeliveryStatus != model.DeliveryStatusSuppressed {
		t.Fatalf("cron_fire should be NO_REPLY+suppress, got %+v", rec)
	}
}
