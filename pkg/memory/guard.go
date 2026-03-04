package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DefaultMaxGetChars = 8000
)

var (
	ErrPathDenied   = errors.New("path denied")
	ErrInvalidRange = errors.New("invalid range")
)

type MemoryFile struct {
	Path  string
	Scope string
}

// ResolvePath validates a memory path and returns normalized relative + absolute path.
func ResolvePath(workspace, rawPath string) (string, string, string, error) {
	p := strings.TrimSpace(rawPath)
	if p == "" {
		return "", "", "", fmt.Errorf("%w: empty path", ErrPathDenied)
	}

	clean := filepath.Clean(filepath.FromSlash(p))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", "", "", fmt.Errorf("%w: outside workspace", ErrPathDenied)
	}

	rel := filepath.ToSlash(clean)
	scope := "private"
	allowed := false
	switch {
	case rel == "MEMORY.md":
		allowed = true
		scope = "public"
	case strings.HasPrefix(rel, "memory/") && strings.HasSuffix(rel, ".md"):
		allowed = true
		if strings.HasPrefix(rel, "memory/public/") {
			scope = "public"
		}
	}
	if !allowed {
		return "", "", "", fmt.Errorf("%w: path not in whitelist", ErrPathDenied)
	}

	abs := filepath.Join(workspace, clean)
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return "", "", "", err
	}
	absPath, err := filepath.Abs(abs)
	if err != nil {
		return "", "", "", err
	}
	relCheck, err := filepath.Rel(workspaceAbs, absPath)
	if err != nil {
		return "", "", "", err
	}
	if relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(os.PathSeparator)) {
		return "", "", "", fmt.Errorf("%w: outside workspace", ErrPathDenied)
	}
	return rel, absPath, scope, nil
}

// ListFiles returns all searchable memory markdown files.
func ListFiles(workspace string) ([]MemoryFile, error) {
	out := make([]MemoryFile, 0, 16)

	memoryRoot := filepath.Join(workspace, "memory")
	if stat, err := os.Stat(memoryRoot); err == nil && stat.IsDir() {
		err = filepath.WalkDir(memoryRoot, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
				return nil
			}
			relPath, err := filepath.Rel(workspace, path)
			if err != nil {
				return nil
			}
			rel, _, scope, err := ResolvePath(workspace, filepath.ToSlash(relPath))
			if err != nil {
				return nil
			}
			out = append(out, MemoryFile{Path: rel, Scope: scope})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	if stat, err := os.Stat(filepath.Join(workspace, "MEMORY.md")); err == nil && !stat.IsDir() {
		out = append(out, MemoryFile{Path: "MEMORY.md", Scope: "public"})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out, nil
}
