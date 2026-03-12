package guardrails

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
)

func WriteReport(path string, report Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFile(path, data)
}

func LoadReport(path string) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return Report{}, err
	}
	return report, nil
}

func BuildBaseline(report Report) Baseline {
	entries := make([]BaselineEntry, 0, len(report.Findings))
	for _, finding := range report.Findings {
		if finding.Status == "shrink_candidate" {
			continue
		}
		entries = append(entries, BaselineEntry{
			RuleID:       finding.RuleID,
			Severity:     finding.Severity,
			File:         finding.File,
			Kind:         finding.Kind,
			StartLine:    finding.StartLine,
			EndLine:      finding.EndLine,
			Symbol:       finding.Symbol,
			Message:      finding.Message,
			Fingerprint:  finding.Fingerprint,
			WhyItMatters: finding.WhyItMatters,
			Remediation:  finding.Remediation,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].File != entries[j].File {
			return entries[i].File < entries[j].File
		}
		if entries[i].RuleID != entries[j].RuleID {
			return entries[i].RuleID < entries[j].RuleID
		}
		if entries[i].StartLine != entries[j].StartLine {
			return entries[i].StartLine < entries[j].StartLine
		}
		return entries[i].Fingerprint < entries[j].Fingerprint
	})
	return Baseline{Findings: entries}
}

func WriteBaseline(path string, baseline Baseline) error {
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFile(path, data)
}

func loadBaseline(path string) (Baseline, error) {
	if path == "" {
		return Baseline{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Baseline{}, nil
		}
		return Baseline{}, err
	}
	var baseline Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return Baseline{}, err
	}
	return baseline, nil
}

func loadAllowlist(path string) (Allowlist, error) {
	if path == "" {
		return Allowlist{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Allowlist{}, nil
		}
		return Allowlist{}, err
	}
	var allowlist Allowlist
	if err := json.Unmarshal(data, &allowlist); err != nil {
		return Allowlist{}, err
	}
	return allowlist, nil
}

func writeFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
