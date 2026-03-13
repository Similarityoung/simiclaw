package guardrails

import (
	"crypto/sha1"
	"encoding/hex"
	"path/filepath"
	"strings"
)

func lineText(lines []string, line int) string {
	if line <= 0 || line > len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[line-1])
}

func newFileFinding(ruleID, severity, rel string, line int, message, why, remediation, fingerprintSeed string) Finding {
	return Finding{
		RuleID:       ruleID,
		Severity:     severity,
		File:         filepath.ToSlash(rel),
		Kind:         "file",
		StartLine:    line,
		Message:      message,
		WhyItMatters: why,
		Remediation:  remediation,
		Fingerprint:  makeFingerprint(ruleID, rel, fingerprintSeed),
		Status:       "new",
	}
}

func newLineFinding(ruleID, severity, rel string, line int, symbol, source, message, why, remediation string) Finding {
	return Finding{
		RuleID:        ruleID,
		Severity:      severity,
		File:          filepath.ToSlash(rel),
		Kind:          "line",
		StartLine:     line,
		Symbol:        symbol,
		Message:       message,
		WhyItMatters:  why,
		Remediation:   remediation,
		Fingerprint:   makeFingerprint(ruleID, rel, symbol, source),
		Status:        "new",
		SourceSnippet: source,
	}
}

func makeFingerprint(parts ...string) string {
	h := sha1.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}
