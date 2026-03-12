package guardrails

import (
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	disallowedTokens = map[string]struct{}{
		"base":    {},
		"helper":  {},
		"helpers": {},
		"manager": {},
		"misc":    {},
		"util":    {},
	}
)

func Check(ctx context.Context, cfg CheckConfig) (Report, error) {
	root := cfg.Root
	if root == "" {
		root = "."
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return Report{}, err
	}

	scope := cfg.Scope
	if scope == "" {
		scope = ScopeRepo
	}

	allowlist, err := loadAllowlist(cfg.AllowlistPath)
	if err != nil {
		return Report{}, err
	}
	baseline, err := loadBaseline(cfg.BaselinePath)
	if err != nil {
		return Report{}, err
	}

	var files []string
	changes := map[string]changedFile{}
	switch scope {
	case ScopeRepo:
		files, err = listTrackedGoFiles(ctx, rootAbs)
	case ScopePR:
		if cfg.BaseSHA == "" || cfg.HeadSHA == "" {
			return Report{}, errors.New("pr scope requires base and head SHA")
		}
		files, changes, err = listChangedGoFiles(ctx, rootAbs, cfg.BaseSHA, cfg.HeadSHA)
	default:
		return Report{}, fmt.Errorf("unsupported scope %q", scope)
	}
	if err != nil {
		return Report{}, err
	}

	findings, err := scanFiles(rootAbs, files, scope, changes, allowlist)
	if err != nil {
		return Report{}, err
	}

	report := Report{
		Scope:       scope,
		BaseSHA:     cfg.BaseSHA,
		HeadSHA:     cfg.HeadSHA,
		GeneratedAt: time.Now().UTC(),
		Findings:    applyBaseline(findings, baseline),
	}
	if scope == ScopeRepo {
		report.Findings = append(report.Findings, shrinkCandidates(findings, baseline)...)
	}
	sortFindings(report.Findings)
	report.Summary = buildSummary(report.Findings)
	return report, nil
}

func scanFiles(root string, files []string, scope Scope, changes map[string]changedFile, allowlist Allowlist) ([]Finding, error) {
	var findings []Finding
	for _, rel := range files {
		rel = filepath.ToSlash(rel)
		if scope == ScopePR {
			if _, ok := changes[rel]; !ok {
				continue
			}
		}
		if strings.HasSuffix(rel, "_test.go") {
			continue
		}
		abs := filepath.Join(root, filepath.FromSlash(rel))
		contentBytes, err := os.ReadFile(abs)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		content := string(contentBytes)
		if isGeneratedGoFile(rel, content) {
			continue
		}
		lines := strings.Split(content, "\n")
		change := changes[rel]

		findings = append(findings, nameTokenFindings(rel, lines)...)
		findings = append(findings, fileLengthFindings(rel, lines, scope)...)

		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, abs, contentBytes, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		funcs := collectFuncRanges(parsed)
		findings = append(findings, callFindings(fset, parsed, rel, lines, funcs, scope, change)...)
		if scope == ScopeRepo {
			findings = append(findings, goRoutineFindings(fset, parsed, rel, lines, funcs)...)
			findings = append(findings, panicFindings(fset, parsed, rel, lines, funcs)...)
		}
	}

	filtered := findings[:0]
	for _, finding := range findings {
		if isAllowed(finding, allowlist) {
			continue
		}
		filtered = append(filtered, finding)
	}
	return filtered, nil
}

func isGeneratedGoFile(path string, content string) bool {
	if strings.HasSuffix(path, ".pb.go") || strings.HasSuffix(path, "_gen.go") || strings.Contains(path, "/mocks/") {
		return true
	}
	limit := content
	if len(limit) > 2048 {
		limit = limit[:2048]
	}
	return strings.Contains(limit, "Code generated") && strings.Contains(limit, "DO NOT EDIT")
}

func nameTokenFindings(rel string, lines []string) []Finding {
	var findings []Finding
	base := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	if tokenText, ok := badToken(base); ok {
		findings = append(findings, newFileFinding("name-token", "error", rel, 1, fmt.Sprintf("avoid disallowed filename token %q", tokenText), "Repository naming guardrails keep AI-generated catch-all names out of the codebase.", "Rename the file to a domain-specific name.", tokenText))
		return findings
	}
	for _, segment := range strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/") {
		if tokenText, ok := badToken(segment); ok {
			findings = append(findings, newFileFinding("name-token", "error", rel, 1, fmt.Sprintf("avoid disallowed directory token %q", tokenText), "Repository naming guardrails keep AI-generated catch-all names out of the codebase.", "Rename the directory or move the file into a domain-specific package.", tokenText))
			return findings
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "package ") {
			continue
		}
		pkgName := strings.TrimSpace(strings.TrimPrefix(trimmed, "package "))
		if tokenText, ok := badToken(pkgName); ok {
			findings = append(findings, newFileFinding("name-token", "error", rel, 1, fmt.Sprintf("avoid disallowed package token %q", tokenText), "Repository naming guardrails keep AI-generated catch-all names out of the codebase.", "Rename the package to a domain-specific name.", tokenText))
		}
		break
	}
	return findings
}

func fileLengthFindings(rel string, lines []string, scope Scope) []Finding {
	lineCount := len(lines)
	switch {
	case lineCount > 600:
		return []Finding{newFileFinding("file-lines", "error", rel, 1, fmt.Sprintf("production Go file is %d lines (> 600)", lineCount), "Very large files are hard to review, encourage dumping-ground growth, and hide AI residue.", "Split the file into smaller, cohesive units or move unrelated logic out.", strconv.Itoa(lineCount))}
	case scope == ScopeRepo && lineCount > 400:
		return []Finding{newFileFinding("file-lines", "warning", rel, 1, fmt.Sprintf("production Go file is %d lines (warning range 401-600)", lineCount), "Large files are refactor hotspots even when they are not yet blocking.", "Consider splitting the file before it grows past the blocking threshold.", strconv.Itoa(lineCount))}
	default:
		return nil
	}
}
