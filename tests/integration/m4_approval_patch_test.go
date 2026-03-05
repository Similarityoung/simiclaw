//go:build integration

package integration

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
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestM4HighRiskPatchApprovalLifecycle(t *testing.T) {
	app := newTestApp(t, true, 32)
	targetPath := filepath.Join(app.Cfg.Workspace, "workflows", "m4_auto.yaml")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("name: old\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	baseHash := hashBytes([]byte("name: old\n"))
	diff := strings.Join([]string{
		"--- a/workflows/m4_auto.yaml",
		"+++ b/workflows/m4_auto.yaml",
		"@@ -1,1 +1,1 @@",
		"-name: old",
		"+name: new",
		"",
	}, "\n")
	req := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv_m4_auto", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv_m4_auto:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload: model.EventPayload{
			Type: "message",
			Text: "/patch",
			Extra: map[string]string{
				"target":                "workflow",
				"target_path":           "workflows/m4_auto.yaml",
				"patch_format":          "unified-diff",
				"diff":                  diff,
				"expected_base_hash":    baseHash,
				"patch_idempotency_key": "m4:auto:1",
			},
		},
	}
	resp, code := postIngest(t, app, req)
	if code != 202 {
		t.Fatalf("ingest expected 202, got %d", code)
	}
	rec := waitEvent(t, app, resp.EventID, 2*time.Second)
	if rec.Status != model.EventStatusCommitted {
		t.Fatalf("expected committed, got %+v", rec)
	}
	if !strings.Contains(rec.AssistantReply, "审批队列") {
		t.Fatalf("assistant reply should mention pending approval, got=%q", rec.AssistantReply)
	}
	raw, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(raw) != "name: old\n" {
		t.Fatalf("target should not be changed before approval, got=%q", string(raw))
	}

	approval := waitApprovalByStatus(t, app, resp.SessionKey, string(model.ApprovalStatusPending), 2*time.Second)
	decisionReq := model.ApprovalDecisionRequest{
		Actor: model.ApprovalActor{Type: "user", ID: "u1"},
		Note:  "approve",
	}
	approveBody, _ := json.Marshal(decisionReq)
	_, status := doRequest(t, app, http.MethodPost, "/v1/approvals/"+approval.ApprovalID+":approve", approveBody)
	if status != 200 {
		t.Fatalf("approve expected 200, got %d", status)
	}
	_, status = doRequest(t, app, http.MethodPost, "/v1/approvals/"+approval.ApprovalID+":approve", approveBody)
	if status != 200 {
		t.Fatalf("duplicate approve expected 200, got %d", status)
	}
	rejectBody, _ := json.Marshal(model.ApprovalDecisionRequest{
		Actor: model.ApprovalActor{Type: "user", ID: "u2"},
		Note:  "reject",
	})
	_, status = doRequest(t, app, http.MethodPost, "/v1/approvals/"+approval.ApprovalID+":reject", rejectBody)
	if status != 409 {
		t.Fatalf("reject after approve expected 409, got %d", status)
	}
	waitFileContent(t, targetPath, "name: new\n", 2*time.Second)
}

func TestM4ExpectedBaseHashMismatchNoPollution(t *testing.T) {
	app := newTestApp(t, true, 32)
	targetPath := filepath.Join(app.Cfg.Workspace, "workflows", "m4_hash.yaml")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("name: old\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	seedResp, code := postIngest(t, app, model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv_m4_hash", ChannelType: "dm", ParticipantID: "u2"},
		IdempotencyKey: "cli:conv_m4_hash:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "seed"},
	})
	if code != 202 {
		t.Fatalf("seed ingest expected 202, got %d", code)
	}
	_ = waitEvent(t, app, seedResp.EventID, 2*time.Second)

	diff := strings.Join([]string{
		"--- a/workflows/m4_hash.yaml",
		"+++ b/workflows/m4_hash.yaml",
		"@@ -1,1 +1,1 @@",
		"-name: old",
		"+name: new",
		"",
	}, "\n")
	approvalID := createApprovalViaAPI(t, app, model.CreateApprovalRequest{
		RunID:           "run_m4_hash_manual",
		SessionKey:      seedResp.SessionKey,
		ActiveSessionID: seedResp.ActiveSessionID,
		ConversationID:  "conv_m4_hash",
		ExpiresAt:       time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		Summary:         "hash mismatch patch",
		Risk:            "high",
		Actions:         []model.Action{{ActionID: "act_hash", ActionIndex: 0, ActionIdempotencyKey: "run_m4_hash_manual:0", Type: "Patch", Risk: "high", RequiresApproval: true, Payload: map[string]any{"target": "workflow", "target_path": "workflows/m4_hash.yaml", "patch_format": "unified-diff", "diff": diff, "expected_base_hash": "sha256:deadbeef", "patch_idempotency_key": "m4:hash:mismatch"}}},
	})
	decideApproval(t, app, approvalID, "approve", 200)

	updated := waitApprovalResultReady(t, app, approvalID, 2*time.Second)
	if len(updated.Results) == 0 || updated.Results[0].OK {
		t.Fatalf("expected failed patch result, got %+v", updated.Results)
	}
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "name: old\n" {
		t.Fatalf("target should stay unchanged on mismatch, got=%q", string(data))
	}
}

func TestM4PatchIdempotencyKeyReusesFirstResult(t *testing.T) {
	app := newTestApp(t, true, 32)
	targetPath := filepath.Join(app.Cfg.Workspace, "workflows", "m4_idem.yaml")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("name: old\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	seedResp, code := postIngest(t, app, model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv_m4_idem", ChannelType: "dm", ParticipantID: "u3"},
		IdempotencyKey: "cli:conv_m4_idem:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "seed"},
	})
	if code != 202 {
		t.Fatalf("seed ingest expected 202, got %d", code)
	}
	_ = waitEvent(t, app, seedResp.EventID, 2*time.Second)

	originalHash := hashBytes([]byte("name: old\n"))
	diff := strings.Join([]string{
		"--- a/workflows/m4_idem.yaml",
		"+++ b/workflows/m4_idem.yaml",
		"@@ -1,1 +1,1 @@",
		"-name: old",
		"+name: new",
		"",
	}, "\n")
	key := "m4:patch:idem"
	firstID := createApprovalViaAPI(t, app, model.CreateApprovalRequest{
		RunID:           "run_m4_idem_1",
		SessionKey:      seedResp.SessionKey,
		ActiveSessionID: seedResp.ActiveSessionID,
		ConversationID:  "conv_m4_idem",
		ExpiresAt:       time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		Summary:         "first patch",
		Risk:            "high",
		Actions:         []model.Action{{ActionID: "act_1", ActionIndex: 0, ActionIdempotencyKey: "run_m4_idem_1:0", Type: "Patch", Risk: "high", RequiresApproval: true, Payload: map[string]any{"target": "workflow", "target_path": "workflows/m4_idem.yaml", "patch_format": "unified-diff", "diff": diff, "expected_base_hash": originalHash, "patch_idempotency_key": key}}},
	})
	decideApproval(t, app, firstID, "approve", 200)
	waitFileContent(t, targetPath, "name: new\n", 2*time.Second)

	secondID := createApprovalViaAPI(t, app, model.CreateApprovalRequest{
		RunID:           "run_m4_idem_2",
		SessionKey:      seedResp.SessionKey,
		ActiveSessionID: seedResp.ActiveSessionID,
		ConversationID:  "conv_m4_idem",
		ExpiresAt:       time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		Summary:         "second patch same key",
		Risk:            "high",
		Actions:         []model.Action{{ActionID: "act_2", ActionIndex: 0, ActionIdempotencyKey: "run_m4_idem_2:0", Type: "Patch", Risk: "high", RequiresApproval: true, Payload: map[string]any{"target": "workflow", "target_path": "workflows/m4_idem.yaml", "patch_format": "unified-diff", "diff": diff, "expected_base_hash": "sha256:bad", "patch_idempotency_key": key}}},
	})
	decideApproval(t, app, secondID, "approve", 200)
	secondRec := waitApprovalResultReady(t, app, secondID, 2*time.Second)
	if len(secondRec.Results) == 0 || !secondRec.Results[0].OK {
		t.Fatalf("second patch should reuse first success, got %+v", secondRec.Results)
	}
	waitFileContent(t, targetPath, "name: new\n", 2*time.Second)
}

func hashBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func waitApprovalByStatus(t *testing.T, app *api.App, sessionKey, status string, timeout time.Duration) model.ApprovalRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		body, code := doRequest(t, app, http.MethodGet, "/v1/approvals?session_key="+sessionKey+"&status="+status, nil)
		if code == 200 {
			var resp model.ApprovalListResponse
			if err := json.Unmarshal(body, &resp); err == nil && len(resp.Items) > 0 {
				return resp.Items[0]
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting approval status=%s session_key=%s", status, sessionKey)
	return model.ApprovalRecord{}
}

func waitFileContent(t *testing.T, path, want string, timeout time.Duration) {
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

func createApprovalViaAPI(t *testing.T, app *api.App, req model.CreateApprovalRequest) string {
	t.Helper()
	body, _ := json.Marshal(req)
	respBody, code := doRequest(t, app, http.MethodPost, "/v1/approvals", body)
	if code != 201 {
		t.Fatalf("create approval expected 201, got %d body=%s", code, string(respBody))
	}
	var resp model.CreateApprovalResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("decode create approval: %v", err)
	}
	return resp.ApprovalID
}

func decideApproval(t *testing.T, app *api.App, approvalID, action string, wantStatus int) {
	t.Helper()
	body, _ := json.Marshal(model.ApprovalDecisionRequest{
		Actor: model.ApprovalActor{Type: "user", ID: "u_decider"},
		Note:  action,
	})
	_, code := doRequest(t, app, http.MethodPost, "/v1/approvals/"+approvalID+":"+action, body)
	if code != wantStatus {
		t.Fatalf("decide %s expected %d, got %d", action, wantStatus, code)
	}
}

func waitApprovalResultReady(t *testing.T, app *api.App, approvalID string, timeout time.Duration) model.ApprovalRecord {
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
	t.Fatalf("timeout waiting approval results for %s", approvalID)
	return model.ApprovalRecord{}
}
