package tools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/skills"
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

func TestNewBashTool(t *testing.T) {
	workspace := t.TempDir()

	tool, err := NewBashTool(workspace)
	if err != nil {
		t.Fatalf("NewBashTool error: %v", err)
	}
	if tool.Name() != bashToolName {
		t.Fatalf("tool name = %q, want %q", tool.Name(), bashToolName)
	}
}

func TestResolveBashWorkingDirectoryRejectsTraversal(t *testing.T) {
	workspace := t.TempDir()

	_, err := resolveBashWorkingDirectory(workspace, "../outside")
	if err == nil {
		t.Fatalf("expected traversal rejection")
	}
	if !errors.Is(err, errFileReadPathDenied) {
		t.Fatalf("expected errFileReadPathDenied, got %v", err)
	}
}

func TestResolveBashWorkingDirectoryRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	externalDir := t.TempDir()

	if err := os.Symlink(externalDir, filepath.Join(workspace, "link")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, err := resolveBashWorkingDirectory(workspace, "link")
	if err == nil {
		t.Fatalf("expected symlink escape rejection")
	}
	if !errors.Is(err, errFileReadPathDenied) {
		t.Fatalf("expected errFileReadPathDenied, got %v", err)
	}
}

func TestResolveBashWorkingDirectoryRejectsMissingDirectory(t *testing.T) {
	workspace := t.TempDir()

	_, err := resolveBashWorkingDirectory(workspace, "missing")
	if err == nil {
		t.Fatalf("expected missing directory rejection")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %q, want does not exist", err.Error())
	}
}

func TestResolveBashTimeout(t *testing.T) {
	def, err := resolveBashTimeout(nil)
	if err != nil {
		t.Fatalf("resolveBashTimeout default error: %v", err)
	}
	if def != bashDefaultTimeout {
		t.Fatalf("default timeout = %v, want %v", def, bashDefaultTimeout)
	}

	zero := 0
	if _, err := resolveBashTimeout(&zero); err == nil {
		t.Fatalf("expected timeout lower bound error")
	}

	tooLarge := int(bashMaxTimeout/time.Second) + 1
	if _, err := resolveBashTimeout(&tooLarge); err == nil {
		t.Fatalf("expected timeout upper bound error")
	}
}

func TestRunBashCommandSuccess(t *testing.T) {
	workspace := t.TempDir()

	out, err := runBashCommand(workspace, "printf 'hello'", 2*time.Second)
	if err != nil {
		t.Fatalf("runBashCommand error: %v", err)
	}
	if out.Stdout != "hello" {
		t.Fatalf("stdout = %q, want %q", out.Stdout, "hello")
	}
	if out.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", out.Stderr)
	}
	if out.ExitCode != 0 {
		t.Fatalf("exit_code = %d, want 0", out.ExitCode)
	}
	if out.TimedOut {
		t.Fatalf("timed_out = true, want false")
	}
}

func TestRunBashCommandTimeout(t *testing.T) {
	workspace := t.TempDir()

	out, err := runBashCommand(workspace, "sleep 2", time.Second)
	if err != nil {
		t.Fatalf("runBashCommand error: %v", err)
	}
	if !out.TimedOut {
		t.Fatalf("timed_out = false, want true")
	}
	if out.ExitCode != -1 {
		t.Fatalf("exit_code = %d, want -1", out.ExitCode)
	}
}

func TestRunBashCommandNonZeroExit(t *testing.T) {
	workspace := t.TempDir()

	out, err := runBashCommand(workspace, "printf 'boom' 1>&2; exit 7", 2*time.Second)
	if err != nil {
		t.Fatalf("runBashCommand error: %v", err)
	}
	if out.ExitCode != 7 {
		t.Fatalf("exit_code = %d, want 7", out.ExitCode)
	}
	if out.Stderr != "boom" {
		t.Fatalf("stderr = %q, want %q", out.Stderr, "boom")
	}
	if out.TimedOut {
		t.Fatalf("timed_out = true, want false")
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

func TestInstructionInjectionAndBashLoopViability(t *testing.T) {
	workspace := t.TempDir()
	skillContent := strings.Join([]string{
		"# OpenClaw Loop Skill",
		"Run command: ./scripts/loop_viability.sh",
		"Expected signal: OPENCLAW_LOOP_OK",
	}, "\n") + "\n"

	skillPath := filepath.Join(workspace, "skills", "ops", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("mkdir skills dir: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte(skillContent), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	scriptPath := filepath.Join(workspace, "scripts", "loop_viability.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("mkdir scripts dir: %v", err)
	}
	script := "#!/usr/bin/env bash\nprintf 'OPENCLAW_LOOP_OK'\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script file: %v", err)
	}

	injection, err := skills.AssembleInstructionInjection(workspace)
	if err != nil {
		t.Fatalf("assemble injection: %v", err)
	}
	if !strings.Contains(injection, skills.SkillsInjectionHeader) {
		t.Fatalf("injection missing header: %q", injection)
	}
	if !strings.Contains(injection, "### ops/SKILL.md") {
		t.Fatalf("injection missing skill path: %q", injection)
	}
	if !strings.Contains(injection, "Run command: ./scripts/loop_viability.sh") {
		t.Fatalf("injection missing instructed command: %q", injection)
	}
	if !strings.Contains(injection, "Expected signal: OPENCLAW_LOOP_OK") {
		t.Fatalf("injection missing expected signal: %q", injection)
	}

	cwd, err := resolveBashWorkingDirectory(workspace, ".")
	if err != nil {
		t.Fatalf("resolve bash cwd: %v", err)
	}
	out, err := runBashCommand(cwd, "./scripts/loop_viability.sh", 2*time.Second)
	if err != nil {
		t.Fatalf("run bash command: %v", err)
	}
	if out.TimedOut {
		t.Fatalf("timed_out = true, want false")
	}
	if out.ExitCode != 0 {
		t.Fatalf("exit_code = %d, want 0 (stderr=%q)", out.ExitCode, out.Stderr)
	}
	if out.Stdout != "OPENCLAW_LOOP_OK" {
		t.Fatalf("stdout = %q, want %q", out.Stdout, "OPENCLAW_LOOP_OK")
	}
}
