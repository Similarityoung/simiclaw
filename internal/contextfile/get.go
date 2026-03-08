package contextfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultMaxGetChars = 8000

func ResolvePath(workspace, rawPath string) (string, string, error) {
	p := strings.TrimSpace(rawPath)
	if p == "" {
		return "", "", fmt.Errorf("path denied: empty path")
	}

	clean := filepath.Clean(filepath.FromSlash(p))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("path denied: outside workspace")
	}

	rel := filepath.ToSlash(clean)
	if !isAllowedPath(rel) {
		return "", "", fmt.Errorf("path denied: path not in whitelist")
	}

	abs := filepath.Join(workspace, clean)
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return "", "", err
	}
	workspaceReal := workspaceAbs
	if resolvedWorkspace, err := filepath.EvalSymlinks(workspaceAbs); err == nil {
		workspaceReal = resolvedWorkspace
	}
	absPath, err := filepath.Abs(abs)
	if err != nil {
		return "", "", err
	}
	inside, err := isWithinWorkspace(workspaceAbs, absPath)
	if err != nil {
		return "", "", err
	}
	if !inside {
		return "", "", fmt.Errorf("path denied: outside workspace")
	}

	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		resolvedAbs, err := filepath.Abs(resolved)
		if err != nil {
			return "", "", err
		}
		inside, err := isWithinWorkspace(workspaceReal, resolvedAbs)
		if err != nil {
			return "", "", err
		}
		if !inside {
			return "", "", fmt.Errorf("path denied: symlink escapes workspace")
		}
		resolvedRel, err := filepath.Rel(workspaceReal, resolvedAbs)
		if err != nil {
			return "", "", err
		}
		if !isAllowedPath(filepath.ToSlash(filepath.Clean(resolvedRel))) {
			return "", "", fmt.Errorf("path denied: symlink target not in whitelist")
		}
	} else if !os.IsNotExist(err) {
		return "", "", err
	}

	return rel, absPath, nil
}

type GetArgs struct {
	Path  string
	Lines []int
}

type GetResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func Get(workspace string, args GetArgs, maxChars int) (GetResult, error) {
	if maxChars <= 0 {
		maxChars = DefaultMaxGetChars
	}
	rel, abs, err := ResolvePath(workspace, args.Path)
	if err != nil {
		return GetResult{}, err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return GetResult{}, err
	}
	raw := strings.ReplaceAll(string(b), "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	start, end, err := normalizeRange(args.Lines, len(lines))
	if err != nil {
		return GetResult{}, err
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
	return GetResult{Path: rel, Content: sb.String()}, nil
}

func isAllowedPath(rel string) bool {
	switch rel {
	case "AGENTS.md", "IDENTITY.md", "USER.md":
		return true
	}
	if strings.HasPrefix(rel, "skills/") && strings.HasSuffix(rel, "/SKILL.md") {
		parts := strings.Split(rel, "/")
		return len(parts) == 3 && parts[1] != ""
	}
	return false
}

func isWithinWorkspace(workspaceAbs, candidateAbs string) (bool, error) {
	relCheck, err := filepath.Rel(workspaceAbs, candidateAbs)
	if err != nil {
		return false, err
	}
	if relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(os.PathSeparator)) {
		return false, nil
	}
	return true, nil
}

func normalizeRange(raw []int, total int) (int, int, error) {
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
