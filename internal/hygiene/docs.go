package hygiene

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
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
		if err := os.WriteFile(cfg.PRBody, []byte(body), 0o644); err != nil {
			return err
		}
	}

	return nil
}

func renderQualityBlock(report guardrails.Report, linksStatus string) (string, error) {
	type view struct {
		GeneratedAt      string
		New              int
		Existing         int
		ShrinkCandidates int
		Warnings         int
		LinksStatus      string
		TopRules         []guardrails.RuleCount
	}
	v := view{
		GeneratedAt:      report.GeneratedAt.UTC().Format(time.RFC3339),
		New:              report.Summary.New,
		Existing:         report.Summary.Existing,
		ShrinkCandidates: report.Summary.ShrinkCandidates,
		Warnings:         report.Summary.Warnings,
		LinksStatus:      statusOrUnknown(linksStatus),
		TopRules:         topRuleCounts(report.Summary.ByRule, 5),
	}
	const tpl = `_This block is maintained by repo hygiene. Do not edit it by hand._

| Metric | Value |
| --- | --- |
| Last Run (UTC) | {{.GeneratedAt}} |
| Guardrails New | {{.New}} |
| Guardrails Existing | {{.Existing}} |
| Shrink Candidates | {{.ShrinkCandidates}} |
| Warning Hotspots | {{.Warnings}} |
| Docs Links | {{.LinksStatus}} |

### Top Rules
{{- if .TopRules }}
{{- range .TopRules }}
- {{ .RuleID }}: {{ .Count }}
{{- end }}
{{- else }}
- No current findings.
{{- end }}`
	return renderTemplate(tpl, v)
}

func renderDebtBlock(report guardrails.Report, linksStatus, linksPreview string) (string, error) {
	type view struct {
		Rows         []ruleRow
		LinksStatus  string
		LinksPreview string
		Shrink       []guardrails.Finding
	}
	rows := summarizeByRule(report.Findings)
	v := view{
		Rows:         rows,
		LinksStatus:  statusOrUnknown(linksStatus),
		LinksPreview: linksPreview,
		Shrink:       shrinkFindings(report.Findings),
	}
	const tpl = `_This block is maintained by repo hygiene. Do not edit it by hand._

| Rule | New | Existing | Shrink Candidates |
| --- | --- | --- | --- |
{{- range .Rows }}
| {{ .RuleID }} | {{ .New }} | {{ .Existing }} | {{ .ShrinkCandidates }} |
{{- end }}

### Shrink Candidates
{{- if .Shrink }}
{{- range .Shrink }}
- {{ .File }}{{ if .StartLine }}:{{ .StartLine }}{{ end }} — {{ .Message }}
{{- end }}
{{- else }}
- No shrink candidates in this run.
{{- end }}

### Docs Links
- Status: {{ .LinksStatus }}
{{- if .LinksPreview }}

~~~text
{{ .LinksPreview }}
~~~
{{- end }}`
	return renderTemplate(tpl, v)
}

func renderPRBody(report guardrails.Report, linksStatus, linksPreview string) (string, error) {
	type view struct {
		GeneratedAt      string
		New              int
		Existing         int
		ShrinkCandidates int
		Warnings         int
		LinksStatus      string
		TopRules         []guardrails.RuleCount
		LinksPreview     string
	}
	v := view{
		GeneratedAt:      report.GeneratedAt.UTC().Format(time.RFC3339),
		New:              report.Summary.New,
		Existing:         report.Summary.Existing,
		ShrinkCandidates: report.Summary.ShrinkCandidates,
		Warnings:         report.Summary.Warnings,
		LinksStatus:      statusOrUnknown(linksStatus),
		TopRules:         topRuleCounts(report.Summary.ByRule, 5),
		LinksPreview:     linksPreview,
	}
	const tpl = `## Repo Hygiene

- Last Run (UTC): {{ .GeneratedAt }}
- Guardrails New: {{ .New }}
- Guardrails Existing: {{ .Existing }}
- Shrink Candidates: {{ .ShrinkCandidates }}
- Warning Hotspots: {{ .Warnings }}
- Docs Links: {{ .LinksStatus }}

### Top Rules
{{- if .TopRules }}
{{- range .TopRules }}
- {{ .RuleID }}: {{ .Count }}
{{- end }}
{{- else }}
- No current findings.
{{- end }}
{{- if .LinksPreview }}

### Docs Links Preview

~~~text
{{ .LinksPreview }}
~~~
{{- end }}`
	return renderTemplate(tpl, v)
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
	return os.WriteFile(path, []byte(updated), 0o644)
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

func renderTemplate(text string, data any) (string, error) {
	tpl, err := template.New("doc").Parse(text)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()) + "\n", nil
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
