package workspacefile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultMaxContextChars = 8000

var allowedContextRootFiles = map[string]struct{}{
	"AGENTS.md":    {},
	"SOUL.md":      {},
	"IDENTITY.md":  {},
	"USER.md":      {},
	"TOOLS.md":     {},
	"BOOTSTRAP.md": {},
	"HEARTBEAT.md": {},
}

type ContextGetArgs struct {
	Path  string
	Lines []int
}

type ContextGetResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type ContextText struct {
	Path         string
	ResolvedPath string
	Content      string
}

func ResolveContextPath(workspace, rawPath string) (string, string, error) {
	candidate, err := resolveWorkspaceCandidate(workspace, rawPath)
	if err != nil {
		return "", "", err
	}
	if !isAllowedContextPath(candidate.RequestPath) {
		return "", "", &Error{Code: CodeForbidden, Message: "path denied: path not in whitelist"}
	}

	resolvedAbs, resolvedRel, ok, err := resolveExistingPath(candidate.WorkspaceReal, candidate.CandidateAbs)
	if err != nil {
		return "", "", err
	}
	if !ok {
		return candidate.RequestPath, candidate.CandidateAbs, nil
	}
	if !isAllowedContextPath(resolvedRel) {
		return "", "", &Error{Code: CodeForbidden, Message: "path denied: symlink target not in whitelist"}
	}

	return candidate.RequestPath, resolvedAbs, nil
}

func GetContext(workspace string, args ContextGetArgs, maxChars int) (ContextGetResult, error) {
	if maxChars <= 0 {
		maxChars = DefaultMaxContextChars
	}
	rel, abs, err := ResolveContextPath(workspace, args.Path)
	if err != nil {
		return ContextGetResult{}, err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return ContextGetResult{}, &Error{Code: CodeNotFound, Message: fmt.Sprintf("file not found: %s", rel)}
		}
		return ContextGetResult{}, err
	}
	raw := strings.ReplaceAll(string(b), "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	start, end, err := normalizeContextRange(args.Lines, len(lines))
	if err != nil {
		return ContextGetResult{}, &Error{Code: CodeInvalidArgument, Message: err.Error()}
	}
	var sb strings.Builder
	for i := start; i <= end; i++ {
		chunk := fmt.Sprintf("%d: %s\n", i, lines[i-1])
		if sb.Len()+len(chunk) > maxChars {
			rest := "...<truncated>"
			if sb.Len()+len(rest) <= maxChars {
				sb.WriteString(rest)
			}
			break
		}
		sb.WriteString(chunk)
	}
	return ContextGetResult{Path: rel, Content: sb.String()}, nil
}

func ReadContextText(workspace, rawPath string) (ContextText, bool, error) {
	rel, abs, err := ResolveContextPath(workspace, rawPath)
	if err != nil {
		return ContextText{}, false, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return ContextText{}, false, nil
		}
		return ContextText{}, false, err
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return ContextText{}, false, nil
	}
	if normalizedAbs, err := filepath.Abs(abs); err == nil {
		abs = normalizedAbs
	}
	return ContextText{
		Path:         rel,
		ResolvedPath: abs,
		Content:      content,
	}, true, nil
}

func isAllowedContextPath(rel string) bool {
	if _, ok := allowedContextRootFiles[rel]; ok {
		return true
	}
	if strings.HasPrefix(rel, "skills/") && strings.HasSuffix(rel, "/SKILL.md") {
		parts := strings.Split(rel, "/")
		return len(parts) == 3 && parts[1] != ""
	}
	return false
}

func normalizeContextRange(raw []int, total int) (int, int, error) {
	if total <= 0 {
		return 0, 0, fmt.Errorf("invalid range: empty file")
	}
	if len(raw) == 0 {
		return 1, total, nil
	}
	if len(raw) != 2 {
		return 0, 0, fmt.Errorf("invalid range: lines must be [start,end]")
	}
	start, end := raw[0], raw[1]
	if start <= 0 || end < start {
		return 0, 0, fmt.Errorf("invalid range: invalid lines")
	}
	if start > total {
		return 0, 0, fmt.Errorf("invalid range: start out of range")
	}
	if end > total {
		end = total
	}
	return start, end, nil
}
