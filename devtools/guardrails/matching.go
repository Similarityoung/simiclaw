package guardrails

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

func badToken(value string) (string, bool) {
	for _, tokenText := range splitTokens(strings.ToLower(value)) {
		if _, ok := disallowedTokens[tokenText]; ok {
			return tokenText, true
		}
	}
	return "", false
}

func splitTokens(value string) []string {
	var parts []string
	var buf strings.Builder
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		parts = append(parts, buf.String())
		buf.Reset()
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			buf.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return parts
}

func isAllowed(finding Finding, allowlist Allowlist) bool {
	for _, entry := range allowlist.Entries {
		if entry.RuleID != finding.RuleID || filepath.ToSlash(entry.File) != finding.File {
			continue
		}
		if entry.Symbol != "" && entry.Symbol != finding.Symbol {
			continue
		}
		if entry.StartLine > 0 {
			end := entry.EndLine
			if end <= 0 {
				end = entry.StartLine
			}
			line := finding.StartLine
			if line < entry.StartLine || line > end {
				continue
			}
		}
		return true
	}
	return false
}

func applyBaseline(findings []Finding, baseline Baseline) ([]Finding, []bool) {
	used := make([]bool, len(baseline.Findings))
	var out []Finding
	for _, finding := range findings {
		matchIdx, match, ok := matchBaseline(finding, baseline, used)
		if ok {
			used[matchIdx] = true
			finding.Status = "existing"
			if finding.Message == "" {
				finding.Message = match.Message
			}
		}
		out = append(out, finding)
	}
	return out, used
}

func shrinkCandidates(baseline Baseline, used []bool) []Finding {
	var shrink []Finding
	for idx, entry := range baseline.Findings {
		if idx < len(used) && used[idx] {
			continue
		}
		shrink = append(shrink, Finding{
			RuleID:       entry.RuleID,
			Severity:     entry.Severity,
			File:         entry.File,
			Kind:         entry.Kind,
			StartLine:    entry.StartLine,
			EndLine:      entry.EndLine,
			Symbol:       entry.Symbol,
			Message:      entry.Message,
			WhyItMatters: entry.WhyItMatters,
			Remediation:  entry.Remediation,
			Fingerprint:  entry.Fingerprint,
			Status:       "shrink_candidate",
		})
	}
	return shrink
}

func matchBaseline(finding Finding, baseline Baseline, used []bool) (int, BaselineEntry, bool) {
	for idx, entry := range baseline.Findings {
		if idx < len(used) && used[idx] {
			continue
		}
		if exactBaselineMatch(entry, finding) {
			return idx, entry, true
		}
	}
	for idx, entry := range baseline.Findings {
		if idx < len(used) && used[idx] {
			continue
		}
		if fingerprintBaselineMatch(entry, finding) {
			return idx, entry, true
		}
	}
	return -1, BaselineEntry{}, false
}

func exactBaselineMatch(entry BaselineEntry, finding Finding) bool {
	if entry.RuleID != finding.RuleID || entry.File != finding.File || entry.Kind != finding.Kind {
		return false
	}
	if entry.Kind == "file" {
		return entry.Fingerprint == finding.Fingerprint
	}
	return entry.StartLine == finding.StartLine && entry.EndLine == finding.EndLine && entry.Symbol == finding.Symbol
}

func fingerprintBaselineMatch(entry BaselineEntry, finding Finding) bool {
	if entry.RuleID != finding.RuleID || entry.File != finding.File || entry.Kind != finding.Kind {
		return false
	}
	if entry.Symbol != finding.Symbol {
		return false
	}
	return entry.Fingerprint == finding.Fingerprint
}

func buildSummary(findings []Finding) Summary {
	summary := Summary{
		ByRule: []RuleCount{},
	}
	byRule := map[string]int{}
	for _, finding := range findings {
		summary.Total++
		switch finding.Status {
		case "new":
			summary.New++
		case "existing":
			summary.Existing++
		case "shrink_candidate":
			summary.ShrinkCandidates++
		}
		if finding.Status == "shrink_candidate" {
			continue
		}
		switch finding.Severity {
		case "warning":
			summary.Warnings++
		default:
			summary.Errors++
		}
		byRule[finding.RuleID]++
	}
	for ruleID, count := range byRule {
		summary.ByRule = append(summary.ByRule, RuleCount{RuleID: ruleID, Count: count})
	}
	sort.Slice(summary.ByRule, func(i, j int) bool {
		if summary.ByRule[i].Count != summary.ByRule[j].Count {
			return summary.ByRule[i].Count > summary.ByRule[j].Count
		}
		return summary.ByRule[i].RuleID < summary.ByRule[j].RuleID
	})
	return summary
}

func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Status != findings[j].Status {
			return findings[i].Status < findings[j].Status
		}
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		if findings[i].StartLine != findings[j].StartLine {
			return findings[i].StartLine < findings[j].StartLine
		}
		if findings[i].RuleID != findings[j].RuleID {
			return findings[i].RuleID < findings[j].RuleID
		}
		return findings[i].Fingerprint < findings[j].Fingerprint
	})
}
