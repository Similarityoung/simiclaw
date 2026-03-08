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

func allowedScopesForChannel(channelType string) map[string]bool {
	switch strings.ToLower(strings.TrimSpace(channelType)) {
	case "dm":
		return map[string]bool{"private": true, "public": true}
	default:
		return map[string]bool{"public": true}
	}
}

// CanAccessScope returns whether a conversation channel can read a memory scope.
func CanAccessScope(channelType, scope string) bool {
	allowed := allowedScopesForChannel(channelType)
	return allowed[strings.ToLower(strings.TrimSpace(scope))]
}

func classifyMemoryPath(rel string) (scope string, allowed bool) {
	switch {
	case rel == "MEMORY.md":
		return "public", true
	case rel == "memory/public/MEMORY.md":
		return "public", true
	case rel == "memory/private/MEMORY.md":
		return "private", true
	case strings.HasPrefix(rel, "memory/public/daily/") && strings.HasSuffix(rel, ".md"):
		return "public", true
	case strings.HasPrefix(rel, "memory/private/daily/") && strings.HasSuffix(rel, ".md"):
		return "private", true
	case strings.HasPrefix(rel, "memory/") && strings.HasSuffix(rel, ".md"):
		if strings.HasPrefix(rel, "memory/public/") {
			return "public", true
		}
		return "private", true
	default:
		return "", false
	}
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
	scope, allowed := classifyMemoryPath(rel)
	if !allowed {
		return "", "", "", fmt.Errorf("%w: path not in whitelist", ErrPathDenied)
	}

	abs := filepath.Join(workspace, clean)
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return "", "", "", err
	}
	workspaceReal := workspaceAbs
	if resolvedWorkspace, err := filepath.EvalSymlinks(workspaceAbs); err == nil {
		workspaceReal = resolvedWorkspace
	}
	absPath, err := filepath.Abs(abs)
	if err != nil {
		return "", "", "", err
	}
	inside, err := isWithinWorkspace(workspaceAbs, absPath)
	if err != nil {
		return "", "", "", err
	}
	if !inside {
		return "", "", "", fmt.Errorf("%w: outside workspace", ErrPathDenied)
	}

	// Prevent symlink escapes: resolved target must stay inside workspace.
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		resolvedAbs, err := filepath.Abs(resolved)
		if err != nil {
			return "", "", "", err
		}
		inside, err := isWithinWorkspace(workspaceReal, resolvedAbs)
		if err != nil {
			return "", "", "", err
		}
		if !inside {
			return "", "", "", fmt.Errorf("%w: symlink escapes workspace", ErrPathDenied)
		}
		resolvedRel, err := filepath.Rel(workspaceReal, resolvedAbs)
		if err != nil {
			return "", "", "", err
		}
		resolvedRel = filepath.ToSlash(filepath.Clean(resolvedRel))
		resolvedScope, resolvedAllowed := classifyMemoryPath(resolvedRel)
		if !resolvedAllowed {
			return "", "", "", fmt.Errorf("%w: symlink target not in whitelist", ErrPathDenied)
		}
		scope = resolvedScope
	} else if !os.IsNotExist(err) {
		return "", "", "", err
	}

	return rel, absPath, scope, nil
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
		if rel, _, scope, err := ResolvePath(workspace, "MEMORY.md"); err == nil {
			out = append(out, MemoryFile{Path: rel, Scope: scope})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out, nil
}
