package workspacefile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveContextPathAllowsRootAndSkillFiles(t *testing.T) {
	workspace := t.TempDir()
	for _, rel := range []string{"AGENTS.md", "skills/alpha/SKILL.md"} {
		path := filepath.Join(workspace, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte("ok\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
		gotRel, abs, err := ResolveContextPath(workspace, rel)
		if err != nil {
			t.Fatalf("ResolveContextPath(%s): %v", rel, err)
		}
		if gotRel != rel {
			t.Fatalf("expected rel %q, got %q", rel, gotRel)
		}
		if abs == "" {
			t.Fatalf("expected abs path for %s", rel)
		}
	}
}

func TestResolveContextPathRejectsNonWhitelistedPath(t *testing.T) {
	workspace := t.TempDir()
	_, _, err := ResolveContextPath(workspace, "memory/public/note.md")
	if err == nil {
		t.Fatal("expected whitelist rejection")
	}
	toolErr, ok := err.(*Error)
	if !ok || toolErr.Code != CodeForbidden {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

func TestResolveContextPathRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(target, []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(workspace, "skills", "alpha", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("mkdir skills dir: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, _, err := ResolveContextPath(workspace, "skills/alpha/SKILL.md")
	if err == nil {
		t.Fatal("expected symlink escape rejection")
	}
	toolErr, ok := err.(*Error)
	if !ok || toolErr.Code != CodeForbidden {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

func TestResolveContextPathAllowsWhitelistedSymlinkTargetInsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "AGENTS.md")
	if err := os.WriteFile(target, []byte("linked\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(workspace, "skills", "alpha", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("mkdir skills dir: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	gotRel, abs, err := ResolveContextPath(workspace, "skills/alpha/SKILL.md")
	if err != nil {
		t.Fatalf("ResolveContextPath: %v", err)
	}
	if gotRel != "skills/alpha/SKILL.md" {
		t.Fatalf("unexpected rel path: %q", gotRel)
	}
	wantAbs, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if abs != wantAbs {
		t.Fatalf("expected resolved abs %q, got %q", wantAbs, abs)
	}
}

func TestGetContextReadsRange(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "AGENTS.md")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	res, err := GetContext(workspace, ContextGetArgs{Path: "AGENTS.md", Lines: []int{2, 3}}, DefaultMaxContextChars)
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}
	if res.Content != "2: b\n3: c\n" {
		t.Fatalf("unexpected ranged content: %q", res.Content)
	}
}

func TestReadContextTextReturnsTrimmedContentAndResolvedPath(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "AGENTS.md")
	if err := os.WriteFile(target, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	text, ok, err := ReadContextText(workspace, "AGENTS.md")
	if err != nil {
		t.Fatalf("ReadContextText: %v", err)
	}
	if !ok {
		t.Fatal("expected readable context text")
	}
	if text.Path != "AGENTS.md" || text.Content != "hello" {
		t.Fatalf("unexpected context text: %+v", text)
	}
	wantAbs, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if text.ResolvedPath != wantAbs {
		t.Fatalf("expected resolved path %q, got %q", wantAbs, text.ResolvedPath)
	}
}
