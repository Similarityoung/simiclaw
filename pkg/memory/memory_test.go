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
