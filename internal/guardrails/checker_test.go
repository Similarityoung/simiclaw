package guardrails

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDiffTracksAddedRanges(t *testing.T) {
	diff := `diff --git a/internal/demo/foo.go b/internal/demo/foo.go
index 123..456 100644
--- a/internal/demo/foo.go
+++ b/internal/demo/foo.go
@@ -10,0 +11,2 @@
+context.Background()
+fmt.Println("x")
@@ -20 +22 @@
-old
+new
`

	changes, err := parseDiff(diff)
	if err != nil {
		t.Fatalf("parseDiff err=%v", err)
	}
	change, ok := changes["internal/demo/foo.go"]
	if !ok {
		t.Fatalf("expected change entry")
	}
	if !change.contains(11) || !change.contains(12) || !change.contains(22) {
		t.Fatalf("expected changed lines to be tracked: %+v", change.ranges)
	}
	if change.contains(10) {
		t.Fatalf("unexpected old line match")
	}
}

func TestMatchBaselineFallsBackToFingerprint(t *testing.T) {
	finding := Finding{
		RuleID:      "context-background",
		File:        "internal/demo/foo.go",
		Kind:        "line",
		StartLine:   20,
		Symbol:      "Demo",
		Fingerprint: "abc123",
	}
	baseline := Baseline{
		Findings: []BaselineEntry{
			{
				RuleID:      "context-background",
				File:        "internal/demo/foo.go",
				Kind:        "line",
				StartLine:   10,
				Symbol:      "Demo",
				Fingerprint: "abc123",
			},
		},
	}
	if _, ok := matchBaseline(finding, baseline); !ok {
		t.Fatalf("expected fingerprint fallback to match")
	}
}

func TestScanFilesFiltersPRLineRulesToChangedLines(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "internal", "demo")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "package demo\n\nfunc Demo() {\n\tcontext.Background()\n\tcontext.Background()\n}\n"
	filePath := filepath.Join(path, "foo.go")
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	findings, err := scanFiles(root, []string{"internal/demo/foo.go"}, ScopePR, map[string]changedFile{
		"internal/demo/foo.go": {
			path:   "internal/demo/foo.go",
			ranges: []lineRange{{start: 5, end: 5}},
		},
	}, Allowlist{})
	if err != nil {
		t.Fatalf("scanFiles err=%v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].StartLine != 5 {
		t.Fatalf("unexpected finding line %d", findings[0].StartLine)
	}
}

func TestIsAllowedMatchesLineRange(t *testing.T) {
	finding := Finding{
		RuleID:    "context-background",
		File:      "internal/demo/foo.go",
		StartLine: 12,
		Symbol:    "Demo",
	}
	allowlist := Allowlist{
		Entries: []AllowEntry{
			{
				RuleID:    "context-background",
				File:      "internal/demo/foo.go",
				StartLine: 10,
				EndLine:   15,
			},
		},
	}
	if !isAllowed(finding, allowlist) {
		t.Fatalf("expected finding to be allowlisted")
	}
}
