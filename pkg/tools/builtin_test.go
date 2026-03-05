package tools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestNewFileWriteTool(t *testing.T) {
	workspace := t.TempDir()

	tool, err := NewFileWriteTool(workspace)
	if err != nil {
		t.Fatalf("NewFileWriteTool error: %v", err)
	}
	if tool.Name() != fileWriteToolName {
		t.Fatalf("tool name = %q, want %q", tool.Name(), fileWriteToolName)
	}
}

func TestNewFileEditTool(t *testing.T) {
	workspace := t.TempDir()

	tool, err := NewFileEditTool(workspace)
	if err != nil {
		t.Fatalf("NewFileEditTool error: %v", err)
	}
	if tool.Name() != fileEditToolName {
		t.Fatalf("tool name = %q, want %q", tool.Name(), fileEditToolName)
	}
}

func TestWriteWorkspaceFileSuccess(t *testing.T) {
	workspace := t.TempDir()

	out, err := writeWorkspaceFile(workspace, FileWriteInput{
		Path:    "docs/note.txt",
		Content: "hello\n",
	})
	if err != nil {
		t.Fatalf("writeWorkspaceFile error: %v", err)
	}
	if out.Path != "docs/note.txt" {
		t.Fatalf("path = %q, want %q", out.Path, "docs/note.txt")
	}
	if out.BytesWritten != len("hello\n") {
		t.Fatalf("bytes_written = %d, want %d", out.BytesWritten, len("hello\n"))
	}

	got, err := os.ReadFile(filepath.Join(workspace, "docs", "note.txt"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != "hello\n" {
		t.Fatalf("content = %q, want %q", string(got), "hello\\n")
	}
}

func TestWriteWorkspaceFileRejectsTraversal(t *testing.T) {
	workspace := t.TempDir()

	_, err := writeWorkspaceFile(workspace, FileWriteInput{
		Path:    "../secret.txt",
		Content: "x",
	})
	if err == nil {
		t.Fatalf("expected traversal rejection")
	}
	if !strings.HasPrefix(err.Error(), "forbidden:") {
		t.Fatalf("error = %q, want forbidden prefix", err.Error())
	}
}

func TestWriteWorkspaceFileRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	externalDir := t.TempDir()

	if err := os.Symlink(externalDir, filepath.Join(workspace, "link")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, err := writeWorkspaceFile(workspace, FileWriteInput{
		Path:    "link/secret.txt",
		Content: "x",
	})
	if err == nil {
		t.Fatalf("expected symlink escape rejection")
	}
	if !strings.HasPrefix(err.Error(), "forbidden:") {
		t.Fatalf("error = %q, want forbidden prefix", err.Error())
	}
}

func TestEditWorkspaceFileSuccess(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "docs", "note.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(path, []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	out, err := editWorkspaceFile(workspace, FileEditInput{
		Path:    "docs/note.txt",
		OldText: "world",
		NewText: "gopher",
	})
	if err != nil {
		t.Fatalf("editWorkspaceFile error: %v", err)
	}
	if out.ReplacedCount != 1 {
		t.Fatalf("replaced_count = %d, want 1", out.ReplacedCount)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read edited file: %v", err)
	}
	if string(got) != "hello gopher\n" {
		t.Fatalf("content = %q, want %q", string(got), "hello gopher\\n")
	}
}

func TestEditWorkspaceFileRejectsTraversal(t *testing.T) {
	workspace := t.TempDir()

	_, err := editWorkspaceFile(workspace, FileEditInput{
		Path:    "../secret.txt",
		OldText: "a",
		NewText: "b",
	})
	if err == nil {
		t.Fatalf("expected traversal rejection")
	}
	if !strings.HasPrefix(err.Error(), "forbidden:") {
		t.Fatalf("error = %q, want forbidden prefix", err.Error())
	}
}

func TestEditWorkspaceFileRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	externalDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(externalDir, "seed.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write external seed file: %v", err)
	}
	if err := os.Symlink(externalDir, filepath.Join(workspace, "link")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, err := editWorkspaceFile(workspace, FileEditInput{
		Path:    "link/seed.txt",
		OldText: "x",
		NewText: "y",
	})
	if err == nil {
		t.Fatalf("expected symlink escape rejection")
	}
	if !strings.HasPrefix(err.Error(), "forbidden:") {
		t.Fatalf("error = %q, want forbidden prefix", err.Error())
	}
}

func TestEditWorkspaceFileNotFoundWhenOldTextMissing(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	_, err := editWorkspaceFile(workspace, FileEditInput{
		Path:    "note.txt",
		OldText: "missing",
		NewText: "x",
	})
	if err == nil {
		t.Fatalf("expected missing old_text error")
	}
	if !strings.HasPrefix(err.Error(), "not_found:") {
		t.Fatalf("error = %q, want not_found prefix", err.Error())
	}
}

func TestEditWorkspaceFileAmbiguousMatchRequiresReplaceAll(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("x x\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	_, err := editWorkspaceFile(workspace, FileEditInput{
		Path:    "note.txt",
		OldText: "x",
		NewText: "y",
	})
	if err == nil {
		t.Fatalf("expected ambiguous match error")
	}
	if !strings.HasPrefix(err.Error(), "invalid_argument:") {
		t.Fatalf("error = %q, want invalid_argument prefix", err.Error())
	}
}

func TestEditWorkspaceFileReplaceAll(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("x x\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	out, err := editWorkspaceFile(workspace, FileEditInput{
		Path:       "note.txt",
		OldText:    "x",
		NewText:    "y",
		ReplaceAll: true,
	})
	if err != nil {
		t.Fatalf("editWorkspaceFile error: %v", err)
	}
	if out.ReplacedCount != 2 {
		t.Fatalf("replaced_count = %d, want 2", out.ReplacedCount)
	}
}
