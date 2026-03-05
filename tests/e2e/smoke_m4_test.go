package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestSmokeM4_ApprovalPatchAndRollback(t *testing.T) {
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	app, err := api.NewApp(cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	app.Start()
	defer app.Stop()

	targetPath := filepath.Join(cfg.Workspace, "workflows", "smoke_m4.yaml")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("name: old\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	diffApply := strings.Join([]string{
		"--- a/workflows/smoke_m4.yaml",
		"+++ b/workflows/smoke_m4.yaml",
		"@@ -1,1 +1,1 @@",
		"-name: old",
		"+name: new",
		"",
	}, "\n")
	req := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "smoke_m4", ChannelType: "dm", ParticipantID: "u4"},
		IdempotencyKey: "cli:smoke_m4:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload: model.EventPayload{
			Type: "message",
			Text: "/patch",
			Extra: map[string]string{
				"target":                "workflow",
				"target_path":           "workflows/smoke_m4.yaml",
				"patch_format":          "unified-diff",
				"diff":                  diffApply,
				"expected_base_hash":    hashBytesE2E([]byte("name: old\n")),
				"patch_idempotency_key": "smoke:m4:apply",
			},
		},
	}
	ingestResp := ingest(t, app, req, 202)
	eventRec := pollEvent(t, app, ingestResp.EventID)
	if !strings.Contains(eventRec.AssistantReply, "审批队列") {
		t.Fatalf("should enter pending approval, got %+v", eventRec)
	}
	approval := waitPendingApprovalE2E(t, app, ingestResp.SessionKey, 2*time.Second)
	approveApprovalE2E(t, app, approval.ApprovalID, "approve", 200)
	waitFileContentE2E(t, targetPath, "name: new\n", 2*time.Second)

	diffRollback := strings.Join([]string{
		"--- a/workflows/smoke_m4.yaml",
		"+++ b/workflows/smoke_m4.yaml",
		"@@ -1,1 +1,2 @@",
		" name: new",
		"+<<<<<<< HEAD",
		"",
	}, "\n")
	req2 := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "smoke_m4", ChannelType: "dm", ParticipantID: "u4"},
		IdempotencyKey: "cli:smoke_m4:2",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload: model.EventPayload{
			Type: "message",
			Text: "/patch",
			Extra: map[string]string{
				"target":                "workflow",
				"target_path":           "workflows/smoke_m4.yaml",
				"patch_format":          "unified-diff",
				"diff":                  diffRollback,
				"expected_base_hash":    hashBytesE2E([]byte("name: new\n")),
				"patch_idempotency_key": "smoke:m4:rollback",
			},
		},
	}
	ingestResp2 := ingest(t, app, req2, 202)
	_ = pollEvent(t, app, ingestResp2.EventID)
	approval2 := waitPendingApprovalE2E(t, app, ingestResp2.SessionKey, 2*time.Second)
	approveApprovalE2E(t, app, approval2.ApprovalID, "approve", 200)
	updated := waitApprovalResultE2E(t, app, approval2.ApprovalID, 2*time.Second)
	if len(updated.Results) == 0 || updated.Results[0].OK {
		t.Fatalf("rollback scenario should fail and rollback, got %+v", updated.Results)
	}
	waitFileContentE2E(t, targetPath, "name: new\n", 2*time.Second)
}

func hashBytesE2E(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func waitPendingApprovalE2E(t *testing.T, app *api.App, sessionKey string, timeout time.Duration) model.ApprovalRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		body, code := doRequest(t, app, http.MethodGet, "/v1/approvals?session_key="+sessionKey+"&status=pending", nil)
		if code == 200 {
			var resp model.ApprovalListResponse
			if err := json.Unmarshal(body, &resp); err == nil && len(resp.Items) > 0 {
				return resp.Items[0]
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting pending approval")
	return model.ApprovalRecord{}
}

func approveApprovalE2E(t *testing.T, app *api.App, approvalID, action string, wantCode int) {
	t.Helper()
	body, _ := json.Marshal(model.ApprovalDecisionRequest{
		Actor: model.ApprovalActor{Type: "user", ID: "u_smoke"},
		Note:  action,
	})
	_, code := doRequest(t, app, http.MethodPost, "/v1/approvals/"+approvalID+":"+action, body)
	if code != wantCode {
		t.Fatalf("%s expected %d, got %d", action, wantCode, code)
	}
}

func waitApprovalResultE2E(t *testing.T, app *api.App, approvalID string, timeout time.Duration) model.ApprovalRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		body, code := doRequest(t, app, http.MethodGet, "/v1/approvals/"+approvalID, nil)
		if code == 200 {
			var rec model.ApprovalRecord
			if err := json.Unmarshal(body, &rec); err == nil && len(rec.Results) > 0 {
				return rec
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting approval result")
	return model.ApprovalRecord{}
}

func waitFileContentE2E(t *testing.T, path, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(path)
		if err == nil && string(b) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	b, _ := os.ReadFile(path)
	t.Fatalf("timeout waiting file content want=%q got=%q", want, string(b))
}
