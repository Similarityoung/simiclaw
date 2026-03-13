package guardrails

import "time"

type Scope string

const (
	ScopePR   Scope = "pr"
	ScopeRepo Scope = "repo"
)

type CheckConfig struct {
	Root          string
	Scope         Scope
	BaseSHA       string
	HeadSHA       string
	BaselinePath  string
	AllowlistPath string
}

type Finding struct {
	RuleID        string `json:"rule_id"`
	Severity      string `json:"severity"`
	File          string `json:"file"`
	Kind          string `json:"kind"`
	StartLine     int    `json:"start_line,omitempty"`
	EndLine       int    `json:"end_line,omitempty"`
	Symbol        string `json:"symbol,omitempty"`
	Message       string `json:"message"`
	WhyItMatters  string `json:"why_it_matters"`
	Remediation   string `json:"remediation"`
	Fingerprint   string `json:"fingerprint"`
	Status        string `json:"status,omitempty"`
	SourceSnippet string `json:"source_snippet,omitempty"`
}

type RuleCount struct {
	RuleID string `json:"rule_id"`
	Count  int    `json:"count"`
}

type Summary struct {
	Total            int         `json:"total"`
	New              int         `json:"new"`
	Existing         int         `json:"existing"`
	ShrinkCandidates int         `json:"shrink_candidates"`
	Warnings         int         `json:"warnings"`
	Errors           int         `json:"errors"`
	ByRule           []RuleCount `json:"by_rule"`
}

type Report struct {
	Scope       Scope     `json:"scope"`
	BaseSHA     string    `json:"base_sha,omitempty"`
	HeadSHA     string    `json:"head_sha,omitempty"`
	GeneratedAt time.Time `json:"generated_at"`
	Summary     Summary   `json:"summary"`
	Findings    []Finding `json:"findings"`
}

type BaselineEntry struct {
	RuleID       string `json:"rule_id"`
	Severity     string `json:"severity,omitempty"`
	File         string `json:"file"`
	Kind         string `json:"kind"`
	StartLine    int    `json:"start_line,omitempty"`
	EndLine      int    `json:"end_line,omitempty"`
	Symbol       string `json:"symbol,omitempty"`
	Message      string `json:"message"`
	Fingerprint  string `json:"fingerprint"`
	WhyItMatters string `json:"why_it_matters,omitempty"`
	Remediation  string `json:"remediation,omitempty"`
}

type Baseline struct {
	Findings []BaselineEntry `json:"findings"`
}

type AllowEntry struct {
	RuleID    string `json:"rule_id"`
	File      string `json:"file"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	Symbol    string `json:"symbol,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type Allowlist struct {
	Entries []AllowEntry `json:"entries"`
}
