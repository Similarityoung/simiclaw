package guardrails

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
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

func WriteReport(path string, report Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
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
	return os.WriteFile(path, data, 0o644)
}

func listTrackedGoFiles(ctx context.Context, root string) ([]string, error) {
	stdout, err := gitOutput(ctx, root, "ls-files", "*.go")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = filepath.ToSlash(line)
		if !strings.HasPrefix(line, "cmd/") && !strings.HasPrefix(line, "internal/") && !strings.HasPrefix(line, "pkg/") {
			continue
		}
		files = append(files, line)
	}
	sort.Strings(files)
	return files, nil
}

func listChangedGoFiles(ctx context.Context, root, base, head string) ([]string, map[string]changedFile, error) {
	nameOnly, err := gitOutput(ctx, root, "diff", "--name-only", "--diff-filter=ACMR", base, head, "--", "*.go")
	if err != nil {
		return nil, nil, err
	}
	diffText, err := gitOutput(ctx, root, "diff", "--unified=0", "--diff-filter=ACMR", base, head, "--", "*.go")
	if err != nil {
		return nil, nil, err
	}
	changes, err := parseDiff(diffText)
	if err != nil {
		return nil, nil, err
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(nameOnly), "\n") {
		line = filepath.ToSlash(strings.TrimSpace(line))
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "cmd/") && !strings.HasPrefix(line, "internal/") && !strings.HasPrefix(line, "pkg/") {
			continue
		}
		files = append(files, line)
	}
	sort.Strings(files)
	return files, changes, nil
}

func gitOutput(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
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

type funcRange struct {
	start  token.Pos
	end    token.Pos
	symbol string
}

func collectFuncRanges(file *ast.File) []funcRange {
	var ranges []funcRange
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		symbol := fn.Name.Name
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			symbol = methodSymbol(fn)
		}
		ranges = append(ranges, funcRange{
			start:  fn.Pos(),
			end:    fn.End(),
			symbol: symbol,
		})
	}
	return ranges
}

func methodSymbol(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	switch expr := fn.Recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if ident, ok := expr.X.(*ast.Ident); ok {
			return "(*" + ident.Name + ")." + fn.Name.Name
		}
	case *ast.Ident:
		return expr.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

func callFindings(fset *token.FileSet, file *ast.File, rel string, lines []string, funcs []funcRange, scope Scope, change changedFile) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		line := fset.Position(call.Pos()).Line
		if scope == ScopePR && !change.contains(line) {
			return true
		}
		symbol := symbolForLine(call.Pos(), funcs)
		sourceLine := lineText(lines, line)
		switch pkgIdent.Name + "." + selector.Sel.Name {
		case "context.Background", "context.TODO":
			findings = append(findings, newLineFinding("context-background", "error", rel, line, symbol, sourceLine, fmt.Sprintf("avoid new %s() in production code", selector.Sel.Name), "Background contexts hide cancellation and timeout boundaries in production code.", "Thread an existing context into this call or document a reviewed entrypoint exception in the allowlist."))
		case "os.WriteFile", "ioutil.WriteFile":
			findings = append(findings, newLineFinding("writefile", "error", rel, line, symbol, sourceLine, "avoid new direct WriteFile calls in production code", "Direct file writes bypass the repository's explicit workspace and persistence boundaries.", "Route the write through the existing owner boundary or add a reviewed allowlist entry for a true boundary owner."))
		default:
			if (pkgIdent.Name == "fmt" || pkgIdent.Name == "log") && strings.HasPrefix(selector.Sel.Name, "Print") {
				findings = append(findings, newLineFinding("print-logging", "error", rel, line, symbol, sourceLine, fmt.Sprintf("avoid new %s.%s calls in production code", pkgIdent.Name, selector.Sel.Name), "Ad hoc printing bypasses structured logging and makes operational diagnostics inconsistent.", "Use the repository's structured logging helpers instead of direct fmt/log printing."))
			}
		}
		return true
	})
	return findings
}

func goRoutineFindings(fset *token.FileSet, file *ast.File, rel string, lines []string, funcs []funcRange) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		stmt, ok := n.(*ast.GoStmt)
		if !ok {
			return true
		}
		line := fset.Position(stmt.Pos()).Line
		symbol := symbolForLine(stmt.Pos(), funcs)
		findings = append(findings, newLineFinding("go-statement", "warning", rel, line, symbol, lineText(lines, line), "review go statement ownership and shutdown behavior", "Background goroutines are a common source of hidden lifecycle and reliability drift.", "Confirm the goroutine has an owner, a stop path, and panic handling."))
		return true
	})
	return findings
}

func panicFindings(fset *token.FileSet, file *ast.File, rel string, lines []string, funcs []funcRange) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "panic" {
			return true
		}
		line := fset.Position(call.Pos()).Line
		symbol := symbolForLine(call.Pos(), funcs)
		findings = append(findings, newLineFinding("panic-call", "warning", rel, line, symbol, lineText(lines, line), "review panic usage in production code", "Panics in production paths can turn recoverable failures into process-wide incidents.", "Prefer explicit error propagation unless this is a deliberately fatal bootstrap path."))
		return true
	})
	return findings
}

func symbolForLine(pos token.Pos, funcs []funcRange) string {
	for _, fn := range funcs {
		if pos >= fn.start && pos <= fn.end {
			return fn.symbol
		}
	}
	return ""
}

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

func applyBaseline(findings []Finding, baseline Baseline) []Finding {
	var out []Finding
	for _, finding := range findings {
		match, ok := matchBaseline(finding, baseline)
		if ok {
			finding.Status = "existing"
			if finding.Message == "" {
				finding.Message = match.Message
			}
		}
		out = append(out, finding)
	}
	return out
}

func shrinkCandidates(findings []Finding, baseline Baseline) []Finding {
	var shrink []Finding
	for _, entry := range baseline.Findings {
		if baselineStillPresent(entry, findings) {
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

func matchBaseline(finding Finding, baseline Baseline) (BaselineEntry, bool) {
	for _, entry := range baseline.Findings {
		if entry.RuleID != finding.RuleID || entry.File != finding.File || entry.Kind != finding.Kind {
			continue
		}
		if entry.StartLine == finding.StartLine && entry.EndLine == finding.EndLine && entry.Symbol == finding.Symbol {
			return entry, true
		}
	}
	for _, entry := range baseline.Findings {
		if entry.RuleID != finding.RuleID || entry.File != finding.File {
			continue
		}
		if entry.Symbol != finding.Symbol {
			continue
		}
		if entry.Fingerprint == finding.Fingerprint {
			return entry, true
		}
	}
	return BaselineEntry{}, false
}

func baselineStillPresent(entry BaselineEntry, findings []Finding) bool {
	for _, finding := range findings {
		if entry.RuleID != finding.RuleID || entry.File != finding.File || entry.Kind != finding.Kind {
			continue
		}
		if entry.StartLine == finding.StartLine && entry.EndLine == finding.EndLine && entry.Symbol == finding.Symbol {
			return true
		}
		if entry.Symbol == finding.Symbol && entry.Fingerprint == finding.Fingerprint {
			return true
		}
	}
	return false
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
