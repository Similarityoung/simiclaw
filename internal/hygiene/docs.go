package hygiene

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/internal/guardrails"
)

const (
	qualityBegin = "<!-- BEGIN:CI_QUALITY_SCORE -->"
	qualityEnd   = "<!-- END:CI_QUALITY_SCORE -->"
	debtBegin    = "<!-- BEGIN:CI_TECH_DEBT -->"
	debtEnd      = "<!-- END:CI_TECH_DEBT -->"
)

type UpdateDocsConfig struct {
	Root               string
	GuardrailsReport   string
	QualityDoc         string
	TechDebtDoc        string
	LinksStatus        string
	LinksReport        string
	PRBody             string
	QualityGeneratedAt time.Time
}

func UpdateDocs(cfg UpdateDocsConfig) error {
	report, err := guardrails.LoadReport(cfg.GuardrailsReport)
	if err != nil {
		return err
	}

	root := cfg.Root
	if root == "" {
		root = "."
	}
	if cfg.QualityDoc == "" {
		cfg.QualityDoc = filepath.Join(root, "docs", "QUALITY_SCORE.md")
	}
	if cfg.TechDebtDoc == "" {
		cfg.TechDebtDoc = filepath.Join(root, "docs", "exec-plans", "tech-debt-tracker.md")
	}

	linksPreview := ""
	if cfg.LinksReport != "" {
		data, err := os.ReadFile(cfg.LinksReport)
		if err == nil {
			linksPreview = trimPreview(string(data), 20)
		}
	}

	qualityBlock, err := renderQualityBlock(report, cfg.LinksStatus)
	if err != nil {
		return err
	}
	debtBlock, err := renderDebtBlock(report, cfg.LinksStatus, linksPreview)
	if err != nil {
		return err
	}
	if err := rewriteMarkedBlock(cfg.QualityDoc, qualityBegin, qualityEnd, qualityBlock); err != nil {
		return err
	}
	if err := rewriteMarkedBlock(cfg.TechDebtDoc, debtBegin, debtEnd, debtBlock); err != nil {
		return err
	}

	if cfg.PRBody != "" {
		body, err := renderPRBody(report, cfg.LinksStatus, linksPreview)
		if err != nil {
			return err
		}
		if err := writeFile(cfg.PRBody, []byte(body)); err != nil {
			return err
		}
	}

	return nil
}

func renderQualityBlock(report guardrails.Report, linksStatus string) (string, error) {
	var b strings.Builder
	fmt.Fprintln(&b, "_This block is maintained by repo hygiene. Do not edit it by hand._")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Metric | Value |")
	fmt.Fprintln(&b, "| --- | --- |")
	fmt.Fprintf(&b, "| Last Run (UTC) | %s |\n", report.GeneratedAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "| Guardrails New | %d |\n", report.Summary.New)
	fmt.Fprintf(&b, "| Guardrails Existing | %d |\n", report.Summary.Existing)
	fmt.Fprintf(&b, "| Shrink Candidates | %d |\n", report.Summary.ShrinkCandidates)
	fmt.Fprintf(&b, "| Warning Hotspots | %d |\n", report.Summary.Warnings)
	fmt.Fprintf(&b, "| Docs Links | %s |\n", statusOrUnknown(linksStatus))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "### Top Rules")
	fmt.Fprintln(&b)
	topRules := topRuleCounts(report.Summary.ByRule, 5)
	if len(topRules) == 0 {
		fmt.Fprintln(&b, "- No current findings.")
	} else {
		for _, rule := range topRules {
			fmt.Fprintf(&b, "- %s: %d\n", rule.RuleID, rule.Count)
		}
	}
	return strings.TrimSpace(b.String()) + "\n", nil
}

func renderDebtBlock(report guardrails.Report, linksStatus, linksPreview string) (string, error) {
	rows := summarizeByRule(report.Findings)
	var b strings.Builder
	fmt.Fprintln(&b, "_This block is maintained by repo hygiene. Do not edit it by hand._")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Rule | New | Existing | Shrink Candidates |")
	fmt.Fprintln(&b, "| --- | --- | --- | --- |")
	for _, row := range rows {
		fmt.Fprintf(&b, "| %s | %d | %d | %d |\n", row.RuleID, row.New, row.Existing, row.ShrinkCandidates)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "### Shrink Candidates")
	fmt.Fprintln(&b)
	shrink := shrinkFindings(report.Findings)
	if len(shrink) == 0 {
		fmt.Fprintln(&b, "- No shrink candidates in this run.")
	} else {
		for _, finding := range shrink {
			if finding.StartLine > 0 {
				fmt.Fprintf(&b, "- %s:%d — %s\n", finding.File, finding.StartLine, finding.Message)
				continue
			}
			fmt.Fprintf(&b, "- %s — %s\n", finding.File, finding.Message)
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "### Docs Links")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Status: %s\n", statusOrUnknown(linksStatus))
	if strings.TrimSpace(linksPreview) != "" {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "~~~text")
		fmt.Fprintln(&b, strings.TrimSpace(linksPreview))
		fmt.Fprintln(&b, "~~~")
	}
	return strings.TrimSpace(b.String()) + "\n", nil
}

func renderPRBody(report guardrails.Report, linksStatus, linksPreview string) (string, error) {
	var b strings.Builder
	fmt.Fprintln(&b, "## Repo Hygiene")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Last Run (UTC): %s\n", report.GeneratedAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Guardrails New: %d\n", report.Summary.New)
	fmt.Fprintf(&b, "- Guardrails Existing: %d\n", report.Summary.Existing)
	fmt.Fprintf(&b, "- Shrink Candidates: %d\n", report.Summary.ShrinkCandidates)
	fmt.Fprintf(&b, "- Warning Hotspots: %d\n", report.Summary.Warnings)
	fmt.Fprintf(&b, "- Docs Links: %s\n", statusOrUnknown(linksStatus))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "### Top Rules")
	fmt.Fprintln(&b)
	topRules := topRuleCounts(report.Summary.ByRule, 5)
	if len(topRules) == 0 {
		fmt.Fprintln(&b, "- No current findings.")
	} else {
		for _, rule := range topRules {
			fmt.Fprintf(&b, "- %s: %d\n", rule.RuleID, rule.Count)
		}
	}
	if strings.TrimSpace(linksPreview) != "" {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "### Docs Links Preview")
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "~~~text")
		fmt.Fprintln(&b, strings.TrimSpace(linksPreview))
		fmt.Fprintln(&b, "~~~")
	}
	return strings.TrimSpace(b.String()) + "\n", nil
}

func rewriteMarkedBlock(path, beginMarker, endMarker, body string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	beginCount := strings.Count(content, beginMarker)
	endCount := strings.Count(content, endMarker)
	if beginCount != 1 || endCount != 1 {
		return fmt.Errorf("%s: expected exactly one %s and %s marker", path, beginMarker, endMarker)
	}
	beginIndex := strings.Index(content, beginMarker)
	endIndex := strings.Index(content, endMarker)
	if beginIndex < 0 || endIndex < 0 || endIndex < beginIndex {
		return fmt.Errorf("%s: invalid marker order", path)
	}
	replacement := beginMarker + "\n" + strings.TrimRight(body, "\n") + "\n" + endMarker
	updated := content[:beginIndex] + replacement + content[endIndex+len(endMarker):]
	if updated == content {
		return nil
	}
	return writeFile(path, []byte(updated))
}

func topRuleCounts(in []guardrails.RuleCount, n int) []guardrails.RuleCount {
	if len(in) <= n {
		return append([]guardrails.RuleCount(nil), in...)
	}
	return append([]guardrails.RuleCount(nil), in[:n]...)
}

type ruleRow struct {
	RuleID           string
	New              int
	Existing         int
	ShrinkCandidates int
}

func summarizeByRule(findings []guardrails.Finding) []ruleRow {
	type counts struct {
		news     int
		existing int
		shrink   int
	}
	m := map[string]counts{}
	for _, finding := range findings {
		row := m[finding.RuleID]
		switch finding.Status {
		case "new":
			row.news++
		case "existing":
			row.existing++
		case "shrink_candidate":
			row.shrink++
		}
		m[finding.RuleID] = row
	}
	rows := make([]ruleRow, 0, len(m))
	for ruleID, count := range m {
		rows = append(rows, ruleRow{
			RuleID:           ruleID,
			New:              count.news,
			Existing:         count.existing,
			ShrinkCandidates: count.shrink,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].RuleID != rows[j].RuleID {
			return rows[i].RuleID < rows[j].RuleID
		}
		return rows[i].New > rows[j].New
	})
	return rows
}

func shrinkFindings(findings []guardrails.Finding) []guardrails.Finding {
	var out []guardrails.Finding
	for _, finding := range findings {
		if finding.Status == "shrink_candidate" {
			out = append(out, finding)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].StartLine < out[j].StartLine
	})
	return out
}

func trimPreview(text string, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func statusOrUnknown(status string) string {
	if strings.TrimSpace(status) == "" {
		return "unknown"
	}
	return status
}
