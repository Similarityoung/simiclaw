package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestContextGetReadsBootstrapFile(t *testing.T) {
	workspace := t.TempDir()
	writeContextFile(t, filepath.Join(workspace, "AGENTS.md"), "line1\nline2\n")

	reg := NewRegistry()
	RegisterBuiltins(reg)
	res := reg.Call(context.Background(), Context{Workspace: workspace}, "context_get", map[string]any{
		"path": "AGENTS.md",
	})

	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	if res.Output["path"] != "AGENTS.md" {
		t.Fatalf("unexpected path output: %+v", res.Output)
	}
	content, _ := res.Output["content"].(string)
	if content != "1: line1\n2: line2\n" {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestContextGetReadsSkillFileRange(t *testing.T) {
	workspace := t.TempDir()
	writeContextFile(t, filepath.Join(workspace, "skills", "alpha", "SKILL.md"), "a\nb\nc\n")

	reg := NewRegistry()
	RegisterBuiltins(reg)
	res := reg.Call(context.Background(), Context{Workspace: workspace}, "context_get", map[string]any{
		"path":  "skills/alpha/SKILL.md",
		"lines": []any{2, 3},
	})

	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	content, _ := res.Output["content"].(string)
	if content != "2: b\n3: c\n" {
		t.Fatalf("unexpected ranged content: %q", content)
	}
}

func TestContextGetReturnsNotFoundForMissingAllowedFile(t *testing.T) {
	workspace := t.TempDir()

	reg := NewRegistry()
	RegisterBuiltins(reg)
	res := reg.Call(context.Background(), Context{Workspace: workspace}, "context_get", map[string]any{
		"path": "USER.md",
	})

	if res.Error == nil || res.Error.Code != model.ErrorCodeNotFound {
		t.Fatalf("expected NOT_FOUND, got %+v", res.Error)
	}
}

func TestContextGetRejectsNonWhitelistedPath(t *testing.T) {
	workspace := t.TempDir()
	writeContextFile(t, filepath.Join(workspace, "memory", "public", "note.md"), "secret\n")

	reg := NewRegistry()
	RegisterBuiltins(reg)
	res := reg.Call(context.Background(), Context{Workspace: workspace}, "context_get", map[string]any{
		"path": "memory/public/note.md",
	})

	if res.Error == nil || res.Error.Code != model.ErrorCodeForbidden {
		t.Fatalf("expected FORBIDDEN, got %+v", res.Error)
	}
}

func TestContextGetRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.md")
	writeContextFile(t, target, "top-secret\n")
	link := filepath.Join(workspace, "skills", "alpha", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("mkdir skills dir: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	reg := NewRegistry()
	RegisterBuiltins(reg)
	res := reg.Call(context.Background(), Context{Workspace: workspace}, "context_get", map[string]any{
		"path": "skills/alpha/SKILL.md",
	})

	if res.Error == nil || res.Error.Code != model.ErrorCodeForbidden {
		t.Fatalf("expected FORBIDDEN for symlink escape, got %+v", res.Error)
	}
}

func writeContextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
