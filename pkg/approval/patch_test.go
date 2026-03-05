package approval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/store"
)

func TestPatchApplyExpectedHashMismatchNoPollution(t *testing.T) {
	workspace := t.TempDir()
	if err := store.InitWorkspace(workspace); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	target := filepath.Join(workspace, "workflows", "flow.yaml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("name: old\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	exec, err := NewPatchExecutor(workspace)
	if err != nil {
		t.Fatalf("new patch executor: %v", err)
	}
	diff := strings.Join([]string{
		"--- a/workflows/flow.yaml",
		"+++ b/workflows/flow.yaml",
		"@@ -1,1 +1,1 @@",
		"-name: old",
		"+name: new",
		"",
	}, "\n")
	res, err := exec.Apply(model.PatchPayload{
		Target:              "workflow",
		TargetPath:          "workflows/flow.yaml",
		PatchFormat:         "unified-diff",
		Diff:                diff,
		ExpectedBaseHash:    "sha256:deadbeef",
		PatchIdempotencyKey: "patch:test:mismatch",
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if res.OK {
		t.Fatalf("expected patch apply failed on hash mismatch, got %+v", res)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "name: old\n" {
		t.Fatalf("target should remain unchanged, got=%q", string(got))
	}
}

func TestPatchApplyIdempotentByPatchKey(t *testing.T) {
	workspace := t.TempDir()
	if err := store.InitWorkspace(workspace); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	target := filepath.Join(workspace, "workflows", "flow.yaml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("name: old\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}

	exec, err := NewPatchExecutor(workspace)
	if err != nil {
		t.Fatalf("new patch executor: %v", err)
	}
	diff := strings.Join([]string{
		"--- a/workflows/flow.yaml",
		"+++ b/workflows/flow.yaml",
		"@@ -1,1 +1,1 @@",
		"-name: old",
		"+name: new",
		"",
	}, "\n")
	key := "patch:test:idem"
	first, err := exec.Apply(model.PatchPayload{
		Target:              "workflow",
		TargetPath:          "workflows/flow.yaml",
		PatchFormat:         "unified-diff",
		Diff:                diff,
		ExpectedBaseHash:    hashRawBytes(raw),
		PatchIdempotencyKey: key,
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if !first.OK {
		t.Fatalf("first apply expected success, got %+v", first)
	}
	second, err := exec.Apply(model.PatchPayload{
		Target:              "workflow",
		TargetPath:          "workflows/flow.yaml",
		PatchFormat:         "unified-diff",
		Diff:                diff,
		ExpectedBaseHash:    "sha256:bad",
		PatchIdempotencyKey: key,
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if !second.OK || !second.FromIdempotent {
		t.Fatalf("second apply should return idempotent success, got %+v", second)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "name: new\n" {
		t.Fatalf("unexpected target content: %q", string(got))
	}
}

func TestPatchGuardRollbackKeepsStableVersion(t *testing.T) {
	workspace := t.TempDir()
	if err := store.InitWorkspace(workspace); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	target := filepath.Join(workspace, "workflows", "flow.yaml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("name: old\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	exec, err := NewPatchExecutor(workspace)
	if err != nil {
		t.Fatalf("new patch executor: %v", err)
	}
	diff := strings.Join([]string{
		"--- a/workflows/flow.yaml",
		"+++ b/workflows/flow.yaml",
		"@@ -1,1 +1,2 @@",
		" name: old",
		"+<<<<<<< HEAD",
		"",
	}, "\n")
	res, err := exec.Apply(model.PatchPayload{
		Target:              "workflow",
		TargetPath:          "workflows/flow.yaml",
		PatchFormat:         "unified-diff",
		Diff:                diff,
		ExpectedBaseHash:    hashRawBytes(raw),
		PatchIdempotencyKey: "patch:test:rollback",
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if res.OK || !res.RolledBack {
		t.Fatalf("expected guard rollback failure, got %+v", res)
	}
	restored, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(restored) != "name: old\n" {
		t.Fatalf("target should be rolled back to stable version, got=%q", string(restored))
	}
}
