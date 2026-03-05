package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSearchScopeAutoByChannelType(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "private"), 0o755); err != nil {
		t.Fatalf("mkdir private: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "public"), 0o755); err != nil {
		t.Fatalf("mkdir public: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "private", "p.md"), []byte("我喜欢 Go\n"), 0o644); err != nil {
		t.Fatalf("write private: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "public", "g.md"), []byte("团队喜欢 Rust\n"), 0o644); err != nil {
		t.Fatalf("write public: %v", err)
	}

	dmRes, err := Search(workspace, SearchArgs{Query: "喜欢", Scope: "auto", ChannelType: "dm", TopK: 10})
	if err != nil {
		t.Fatalf("search dm: %v", err)
	}
	if len(dmRes.Hits) < 2 {
		t.Fatalf("dm auto should include private+public hits, got=%d", len(dmRes.Hits))
	}

	groupRes, err := Search(workspace, SearchArgs{Query: "喜欢", Scope: "auto", ChannelType: "group", TopK: 10})
	if err != nil {
		t.Fatalf("search group: %v", err)
	}
	for _, hit := range groupRes.Hits {
		if hit.Scope != "public" {
			t.Fatalf("group auto should include only public hits, got hit=%+v", hit)
		}
	}
}

func TestSearchGroupIgnoresMemoryMDSymlinkToPrivate(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "private"), 0o755); err != nil {
		t.Fatalf("mkdir private: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "private", "secret.md"), []byte("private secret 42\n"), 0o644); err != nil {
		t.Fatalf("write private: %v", err)
	}
	if err := os.Symlink(filepath.Join("memory", "private", "secret.md"), filepath.Join(workspace, "MEMORY.md")); err != nil {
		t.Fatalf("symlink MEMORY.md: %v", err)
	}

	groupRes, err := Search(workspace, SearchArgs{
		Query:       "secret",
		Scope:       "auto",
		ChannelType: "group",
		TopK:        10,
	})
	if err != nil {
		t.Fatalf("search group: %v", err)
	}
	if len(groupRes.Hits) != 0 {
		t.Fatalf("group auto should not include private hit via MEMORY.md symlink, got=%+v", groupRes.Hits)
	}

	dmRes, err := Search(workspace, SearchArgs{
		Query:       "secret",
		Scope:       "auto",
		ChannelType: "dm",
		TopK:        10,
	})
	if err != nil {
		t.Fatalf("search dm: %v", err)
	}
	if len(dmRes.Hits) == 0 {
		t.Fatalf("dm auto should include private hit, got=%+v", dmRes.Hits)
	}
}

func TestGetRejectsTraversalPath(t *testing.T) {
	workspace := t.TempDir()
	_, err := Get(workspace, GetArgs{Path: "../../etc/passwd", Lines: []int{1, 2}}, DefaultMaxGetChars)
	if err == nil {
		t.Fatalf("expected traversal path error")
	}
	if !errors.Is(err, ErrPathDenied) {
		t.Fatalf("expected ErrPathDenied, got %v", err)
	}
}

func TestGetRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "private"), 0o755); err != nil {
		t.Fatalf("mkdir memory/private: %v", err)
	}
	target := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(target, []byte("top-secret\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(workspace, "memory", "private", "escape.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := Get(workspace, GetArgs{Path: "memory/private/escape.md", Lines: []int{1, 1}}, DefaultMaxGetChars)
	if err == nil {
		t.Fatalf("expected symlink escape error")
	}
	if !errors.Is(err, ErrPathDenied) {
		t.Fatalf("expected ErrPathDenied, got %v", err)
	}
}

func TestResolvePathUsesResolvedTargetScope(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "private"), 0o755); err != nil {
		t.Fatalf("mkdir private: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "public"), 0o755); err != nil {
		t.Fatalf("mkdir public: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "private", "secret.md"), []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("write private secret: %v", err)
	}
	link := filepath.Join(workspace, "memory", "public", "alias.md")
	if err := os.Symlink(filepath.Join("..", "private", "secret.md"), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, _, scope, err := ResolvePath(workspace, "memory/public/alias.md")
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}
	if scope != "private" {
		t.Fatalf("expected resolved scope private, got %q", scope)
	}
}

func TestGetRejectsSymlinkToNonWhitelistedTarget(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "public"), 0o755); err != nil {
		t.Fatalf("mkdir public: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "runtime"), 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	target := filepath.Join(workspace, "runtime", "secret.txt")
	if err := os.WriteFile(target, []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(workspace, "memory", "public", "alias.md")
	if err := os.Symlink(filepath.Join("..", "..", "runtime", "secret.txt"), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := Get(workspace, GetArgs{Path: "memory/public/alias.md", Lines: []int{1, 1}}, DefaultMaxGetChars)
	if err == nil {
		t.Fatalf("expected symlink whitelist error")
	}
	if !errors.Is(err, ErrPathDenied) {
		t.Fatalf("expected ErrPathDenied, got %v", err)
	}
}

func TestGetRangeAndTruncate(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory"), 0o755); err != nil {
		t.Fatalf("mkdir memory: %v", err)
	}
	path := filepath.Join(workspace, "memory", "a.md")
	data := strings.Repeat("line\n", 200)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	res, err := Get(workspace, GetArgs{Path: "memory/a.md", Lines: []int{1, 200}}, 30)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(res.Content) > 30 {
		t.Fatalf("content should obey max chars, got len=%d content=%q", len(res.Content), res.Content)
	}
	if strings.Contains(res.Content, "50: line") {
		t.Fatalf("content should be truncated by max chars, got=%q", res.Content)
	}
}

func TestWriterDailyAndCurated(t *testing.T) {
	workspace := t.TempDir()
	w := NewWriter(workspace)
	now := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)

	dailyPath, err := w.WriteDaily("test", "记住我喜欢 Go", now)
	if err != nil {
		t.Fatalf("write daily: %v", err)
	}
	if dailyPath != "memory/2026-03-05.md" {
		t.Fatalf("unexpected daily path: %s", dailyPath)
	}
	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(dailyPath))); err != nil {
		t.Fatalf("daily file should exist: %v", err)
	}

	curatedPath, err := w.WriteCurated("我喜欢 Go", now)
	if err != nil {
		t.Fatalf("write curated: %v", err)
	}
	if curatedPath != "MEMORY.md" {
		t.Fatalf("unexpected curated path: %s", curatedPath)
	}
	curatedData, err := os.ReadFile(filepath.Join(workspace, curatedPath))
	if err != nil {
		t.Fatalf("read curated: %v", err)
	}
	if !strings.Contains(string(curatedData), "我喜欢 Go") {
		t.Fatalf("curated content mismatch: %s", string(curatedData))
	}
}
