package hygiene

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/similarityyoung/simiclaw/devtools/guardrails"
)

func TestRewriteMarkedBlockRequiresSingleMarkerPair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "QUALITY.md")
	if err := os.WriteFile(path, []byte("missing markers\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := rewriteMarkedBlock(path, qualityBegin, qualityEnd, "body"); err == nil {
		t.Fatalf("expected marker validation error")
	}
}

func TestUpdateDocsRewritesOnlyManagedBlocks(t *testing.T) {
	root := t.TempDir()
	quality := filepath.Join(root, "QUALITY.md")
	debt := filepath.Join(root, "DEBT.md")
	reportPath := filepath.Join(root, "report.json")
	report := guardrails.Report{
		Summary: guardrails.Summary{
			New:              1,
			Existing:         2,
			ShrinkCandidates: 1,
			Warnings:         1,
			ByRule: []guardrails.RuleCount{
				{RuleID: "context-background", Count: 2},
			},
		},
		Findings: []guardrails.Finding{
			{RuleID: "context-background", File: "internal/demo/foo.go", StartLine: 10, Message: "existing", Status: "existing"},
			{RuleID: "context-background", File: "internal/demo/foo.go", StartLine: 11, Message: "new", Status: "new"},
			{RuleID: "writefile", File: "internal/demo/bar.go", StartLine: 20, Message: "shrunk", Status: "shrink_candidate"},
		},
	}
	if err := guardrails.WriteReport(reportPath, report); err != nil {
		t.Fatalf("write report: %v", err)
	}
	qualityContent := "# Quality\n\nIntro\n\n" + qualityBegin + "\nold\n" + qualityEnd + "\n"
	debtContent := "# Debt\n\nIntro\n\n" + debtBegin + "\nold\n" + debtEnd + "\n"
	if err := os.WriteFile(quality, []byte(qualityContent), 0o644); err != nil {
		t.Fatalf("write quality: %v", err)
	}
	if err := os.WriteFile(debt, []byte(debtContent), 0o644); err != nil {
		t.Fatalf("write debt: %v", err)
	}

	if err := UpdateDocs(UpdateDocsConfig{
		GuardrailsReport: reportPath,
		QualityDoc:       quality,
		TechDebtDoc:      debt,
		LinksStatus:      "failed",
	}); err != nil {
		t.Fatalf("UpdateDocs err=%v", err)
	}

	qualityOut, err := os.ReadFile(quality)
	if err != nil {
		t.Fatalf("read quality: %v", err)
	}
	if !strings.Contains(string(qualityOut), "Guardrails New | 1") {
		t.Fatalf("expected updated quality block, got:\n%s", string(qualityOut))
	}
	if !strings.Contains(string(qualityOut), "Intro") {
		t.Fatalf("expected non-managed content to stay intact")
	}
	debtOut, err := os.ReadFile(debt)
	if err != nil {
		t.Fatalf("read debt: %v", err)
	}
	if !strings.Contains(string(debtOut), "shrink_candidate") && !strings.Contains(string(debtOut), "Shrink Candidates") {
		t.Fatalf("expected updated debt block, got:\n%s", string(debtOut))
	}
}
