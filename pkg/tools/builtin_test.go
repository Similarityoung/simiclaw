package tools

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewFileReadTool(t *testing.T) {
	workspace := t.TempDir()

	tool, err := NewFileReadTool(workspace)
	if err != nil {
		t.Fatalf("NewFileReadTool error: %v", err)
	}
	if tool.Name() != fileReadToolName {
		t.Fatalf("tool name = %q, want %q", tool.Name(), fileReadToolName)
	}
}

func TestResolveFileReadPathRejectsTraversal(t *testing.T) {
	workspace := t.TempDir()

	_, _, err := resolveFileReadPath(workspace, "../secret.txt")
	if err == nil {
		t.Fatalf("expected traversal rejection")
	}
	if !errors.Is(err, errFileReadPathDenied) {
		t.Fatalf("expected errFileReadPathDenied, got %v", err)
	}
}

func TestResolveFileReadPathRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	externalDir := t.TempDir()
	externalFile := filepath.Join(externalDir, "secret.txt")
	if err := os.WriteFile(externalFile, []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("write external file: %v", err)
	}

	symlinkPath := filepath.Join(workspace, "alias.txt")
	if err := os.Symlink(externalFile, symlinkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, _, err := resolveFileReadPath(workspace, "alias.txt")
	if err == nil {
		t.Fatalf("expected symlink escape rejection")
	}
	if !errors.Is(err, errFileReadPathDenied) {
		t.Fatalf("expected errFileReadPathDenied, got %v", err)
	}
}

func TestResolveFileReadPathAllowsWorkspaceFile(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	insideFile := filepath.Join(workspace, "docs", "a.txt")
	if err := os.WriteFile(insideFile, []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write inside file: %v", err)
	}

	rel, abs, err := resolveFileReadPath(workspace, "docs/a.txt")
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}
	if rel != "docs/a.txt" {
		t.Fatalf("rel path = %q, want docs/a.txt", rel)
	}
	if abs != insideFile {
		t.Fatalf("abs path = %q, want %q", abs, insideFile)
	}
}

func TestBuildFileReadOutput(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	out, err := buildFileReadOutput("note.txt", path, 2, 1)
	if err != nil {
		t.Fatalf("buildFileReadOutput error: %v", err)
	}
	if out.Content != "line2\n" {
		t.Fatalf("content = %q, want %q", out.Content, "line2\\n")
	}
	if out.StartLine != 2 || out.EndLine != 2 {
		t.Fatalf("line range = %d-%d, want 2-2", out.StartLine, out.EndLine)
	}
	if out.TotalLines != 3 {
		t.Fatalf("total lines = %d, want 3", out.TotalLines)
	}
	if !out.Truncated {
		t.Fatalf("expected truncated=true")
	}
}
