package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type GetArgs struct {
	Path  string
	Lines []int
}

type GetResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type TextFile struct {
	Path         string
	ResolvedPath string
	Visibility   string
	Kind         string
	Content      string
}

func ReadText(workspace, rawPath string, allowedVisibilities map[string]bool) (TextFile, bool, error) {
	rel, abs, info, err := ResolvePathInfo(workspace, rawPath)
	if err != nil {
		return TextFile{}, false, err
	}
	if len(allowedVisibilities) > 0 && !allowedVisibilities[NormalizeVisibility(info.Visibility)] {
		return TextFile{}, false, nil
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return TextFile{}, false, nil
		}
		return TextFile{}, false, err
	}
	content := strings.TrimSpace(string(b))
	if content == "" {
		return TextFile{}, false, nil
	}
	if normalizedAbs, err := filepath.Abs(abs); err == nil {
		abs = normalizedAbs
	}
	return TextFile{
		Path:         rel,
		ResolvedPath: abs,
		Visibility:   info.Visibility,
		Kind:         info.Kind,
		Content:      content,
	}, true, nil
}

func Get(workspace string, args GetArgs, maxChars int) (GetResult, error) {
	if maxChars <= 0 {
		maxChars = DefaultMaxGetChars
	}

	rel, abs, _, err := ResolvePath(workspace, args.Path)
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

func normalizeRange(raw []int, total int) (int, int, error) {
	if total <= 0 {
		return 0, 0, fmt.Errorf("%w: empty file", ErrInvalidRange)
	}
	if len(raw) == 0 {
		return 1, total, nil
	}
	if len(raw) != 2 {
		return 0, 0, fmt.Errorf("%w: lines must be [start,end]", ErrInvalidRange)
	}
	start, end := raw[0], raw[1]
	if start <= 0 || end < start {
		return 0, 0, fmt.Errorf("%w: invalid lines", ErrInvalidRange)
	}
	if start > total {
		return 0, 0, fmt.Errorf("%w: start out of range", ErrInvalidRange)
	}
	if end > total {
		end = total
	}
	return start, end, nil
}
