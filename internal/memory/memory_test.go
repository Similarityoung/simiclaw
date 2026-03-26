package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestSearchVisibilityAutoByChannelType(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "private"), 0o755); err != nil {
		t.Fatalf("mkdir private: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "public", "daily"), 0o755); err != nil {
		t.Fatalf("mkdir public: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "private", "MEMORY.md"), []byte("我喜欢 Go\n"), 0o644); err != nil {
		t.Fatalf("write private: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "public", "daily", "2026-03-05.md"), []byte("团队喜欢 Rust\n"), 0o644); err != nil {
		t.Fatalf("write public: %v", err)
	}

	dmRes, err := Search(workspace, SearchArgs{Query: "喜欢", Visibility: "auto", Kind: "any", ChannelType: "dm", TopK: 10})
	if err != nil {
		t.Fatalf("search dm: %v", err)
	}
	if len(dmRes.Hits) < 2 {
		t.Fatalf("dm auto should include private+public hits, got=%d", len(dmRes.Hits))
	}

	groupRes, err := Search(workspace, SearchArgs{Query: "喜欢", Visibility: "auto", Kind: "any", ChannelType: "group", TopK: 10})
	if err != nil {
		t.Fatalf("search group: %v", err)
	}
	for _, hit := range groupRes.Hits {
		if hit.Visibility != VisibilityPublic {
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
		Visibility:  "auto",
		Kind:        "any",
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
		Visibility:  "auto",
		Kind:        "any",
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

func TestSearchPreviewIsUTF8Safe(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "public"), 0o755); err != nil {
		t.Fatalf("mkdir public: %v", err)
	}

	long := strings.Repeat("你", 140)
	if err := os.WriteFile(filepath.Join(workspace, "memory", "public", "utf8.md"), []byte("前缀"+long+"\n"), 0o644); err != nil {
		t.Fatalf("write utf8 file: %v", err)
	}

	res, err := Search(workspace, SearchArgs{
		Query:       "你",
		Visibility:  "public",
		Kind:        "any",
		ChannelType: "group",
		TopK:        1,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("expected one hit, got=%d", len(res.Hits))
	}

	preview := res.Hits[0].Preview
	if !utf8.ValidString(preview) {
		t.Fatalf("preview should be valid UTF-8, got=%q", preview)
	}
	if !strings.HasSuffix(preview, "...") {
		t.Fatalf("preview should be truncated with ellipsis, got=%q", preview)
	}
	if len([]rune(preview)) != 123 {
		t.Fatalf("preview rune length should be 123 (120+...), got=%d", len([]rune(preview)))
	}
}

func TestSearchVisibilityAndKindMatrix(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "public", "daily"), 0o755); err != nil {
		t.Fatalf("mkdir public daily: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "private", "daily"), 0o755); err != nil {
		t.Fatalf("mkdir private daily: %v", err)
	}
	write := func(rel, content string) {
		t.Helper()
		abs := filepath.Join(workspace, filepath.FromSlash(rel))
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	write("memory/public/MEMORY.md", "shared token\n")
	write("memory/public/daily/2026-03-05.md", "shared token\n")
	write("memory/private/MEMORY.md", "shared token\n")
	write("memory/private/daily/2026-03-05.md", "shared token\n")

	tests := []struct {
		name        string
		channelType string
		visibility  string
		kind        string
		wantCount   int
		wantKinds   map[string]bool
		wantVis     map[string]bool
	}{
		{name: "group public any", channelType: "group", visibility: "public", kind: "any", wantCount: 2, wantKinds: map[string]bool{"curated": true, "daily": true}, wantVis: map[string]bool{VisibilityPublic: true}},
		{name: "group private blocked", channelType: "group", visibility: "private", kind: "any", wantCount: 0},
		{name: "dm private curated", channelType: "dm", visibility: "private", kind: "curated", wantCount: 1, wantKinds: map[string]bool{"curated": true}, wantVis: map[string]bool{VisibilityPrivate: true}},
		{name: "dm auto daily", channelType: "dm", visibility: "auto", kind: "daily", wantCount: 2, wantKinds: map[string]bool{"daily": true}, wantVis: map[string]bool{VisibilityPrivate: true, VisibilityPublic: true}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Search(workspace, SearchArgs{Query: "shared token", Visibility: tc.visibility, Kind: tc.kind, ChannelType: tc.channelType, TopK: 10})
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if len(res.Hits) != tc.wantCount {
				t.Fatalf("unexpected hit count=%d hits=%+v", len(res.Hits), res.Hits)
			}
			for _, hit := range res.Hits {
				if tc.wantKinds != nil && !tc.wantKinds[hit.Kind] {
					t.Fatalf("unexpected kind %q in hit %+v", hit.Kind, hit)
				}
				if tc.wantVis != nil && !tc.wantVis[hit.Visibility] {
					t.Fatalf("unexpected visibility %q in hit %+v", hit.Visibility, hit)
				}
			}
		})
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

	dailyPath, err := w.WriteDaily("test", "记住我喜欢 Go", now, VisibilityPrivate)
	if err != nil {
		t.Fatalf("write daily: %v", err)
	}
	if dailyPath != "memory/private/daily/2026-03-05.md" {
		t.Fatalf("unexpected daily path: %s", dailyPath)
	}
	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(dailyPath))); err != nil {
		t.Fatalf("daily file should exist: %v", err)
	}

	curatedPath, err := w.WriteCurated("我喜欢 Go", now, VisibilityPublic)
	if err != nil {
		t.Fatalf("write curated: %v", err)
	}
	if curatedPath != "memory/public/MEMORY.md" {
		t.Fatalf("unexpected curated path: %s", curatedPath)
	}
	curatedData, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(curatedPath)))
	if err != nil {
		t.Fatalf("read curated: %v", err)
	}
	if !strings.Contains(string(curatedData), "我喜欢 Go") {
		t.Fatalf("curated content mismatch: %s", string(curatedData))
	}
}

func TestGetAllowsLegacyRootCuratedPath(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "MEMORY.md"), []byte("legacy\n"), 0o644); err != nil {
		t.Fatalf("write legacy curated: %v", err)
	}

	res, err := Get(workspace, GetArgs{Path: "MEMORY.md", Lines: []int{1, 1}}, DefaultMaxGetChars)
	if err != nil {
		t.Fatalf("get legacy curated: %v", err)
	}
	if res.Path != "MEMORY.md" {
		t.Fatalf("unexpected legacy path: %s", res.Path)
	}
	if !strings.Contains(res.Content, "legacy") {
		t.Fatalf("unexpected legacy content: %q", res.Content)
	}
}

func TestReadTextRespectsAllowedVisibility(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "private"), 0o755); err != nil {
		t.Fatalf("mkdir private: %v", err)
	}
	target := filepath.Join(workspace, "memory", "private", "MEMORY.md")
	if err := os.WriteFile(target, []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("write private memory: %v", err)
	}

	if _, ok, err := ReadText(workspace, "memory/private/MEMORY.md", map[string]bool{VisibilityPublic: true}); err != nil {
		t.Fatalf("ReadText public-only: %v", err)
	} else if ok {
		t.Fatal("expected private memory to be filtered out")
	}

	text, ok, err := ReadText(workspace, "memory/private/MEMORY.md", map[string]bool{VisibilityPrivate: true})
	if err != nil {
		t.Fatalf("ReadText private: %v", err)
	}
	if !ok {
		t.Fatal("expected private memory to be readable")
	}
	if text.Visibility != VisibilityPrivate || text.Kind != "curated" || text.Content != "secret" {
		t.Fatalf("unexpected text metadata: %+v", text)
	}
	wantAbs := resolvedTargetPath(t, target)
	if text.ResolvedPath != wantAbs {
		t.Fatalf("expected resolved path %q, got %q", wantAbs, text.ResolvedPath)
	}
}

func TestReadTextReturnsResolvedTargetPathForSymlink(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "public"), 0o755); err != nil {
		t.Fatalf("mkdir public: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "public", "MEMORY.md"), []byte("shared\n"), 0o644); err != nil {
		t.Fatalf("write public memory: %v", err)
	}
	if err := os.Symlink(filepath.Join("memory", "public", "MEMORY.md"), filepath.Join(workspace, "MEMORY.md")); err != nil {
		t.Fatalf("symlink legacy memory: %v", err)
	}

	text, ok, err := ReadText(workspace, "MEMORY.md", map[string]bool{VisibilityPublic: true})
	if err != nil {
		t.Fatalf("ReadText legacy symlink: %v", err)
	}
	if !ok {
		t.Fatal("expected legacy symlink memory to be readable")
	}
	wantAbs := resolvedTargetPath(t, filepath.Join(workspace, "memory", "public", "MEMORY.md"))
	if text.ResolvedPath != wantAbs {
		t.Fatalf("expected resolved symlink target %q, got %q", wantAbs, text.ResolvedPath)
	}
}

func resolvedTargetPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve target path: %v", err)
	}
	return abs
}

func TestVisibilityForChannel(t *testing.T) {
	if got := VisibilityForChannel("dm"); got != VisibilityPrivate {
		t.Fatalf("dm should map to private visibility, got %q", got)
	}
	if got := VisibilityForChannel("group"); got != VisibilityPublic {
		t.Fatalf("group should map to public visibility, got %q", got)
	}
}
