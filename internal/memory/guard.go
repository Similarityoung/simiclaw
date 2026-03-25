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
	Path       string
	Visibility string
	Kind       string
}

type PathInfo struct {
	Visibility string
	Kind       string
}

func allowedVisibilitiesForChannel(channelType string) map[string]bool {
	switch strings.ToLower(strings.TrimSpace(channelType)) {
	case "dm":
		return map[string]bool{VisibilityPrivate: true, VisibilityPublic: true}
	default:
		return map[string]bool{VisibilityPublic: true}
	}
}

func CanAccessVisibility(channelType, visibility string) bool {
	allowed := allowedVisibilitiesForChannel(channelType)
	return allowed[strings.ToLower(strings.TrimSpace(visibility))]
}

// CanAccessScope keeps the existing name for callers that still think in scope/public/private terms.
func CanAccessScope(channelType, scope string) bool {
	return CanAccessVisibility(channelType, scope)
}

func describeMemoryPath(rel string) (info PathInfo, allowed bool) {
	switch {
	case rel == "MEMORY.md":
		return PathInfo{Visibility: VisibilityPublic, Kind: "curated"}, true
	case rel == "memory/public/MEMORY.md":
		return PathInfo{Visibility: VisibilityPublic, Kind: "curated"}, true
	case rel == "memory/private/MEMORY.md":
		return PathInfo{Visibility: VisibilityPrivate, Kind: "curated"}, true
	case strings.HasPrefix(rel, "memory/public/daily/") && strings.HasSuffix(rel, ".md"):
		return PathInfo{Visibility: VisibilityPublic, Kind: "daily"}, true
	case strings.HasPrefix(rel, "memory/private/daily/") && strings.HasSuffix(rel, ".md"):
		return PathInfo{Visibility: VisibilityPrivate, Kind: "daily"}, true
	case strings.HasPrefix(rel, "memory/public/") && strings.HasSuffix(rel, ".md"):
		return PathInfo{Visibility: VisibilityPublic, Kind: "daily"}, true
	case strings.HasPrefix(rel, "memory/private/") && strings.HasSuffix(rel, ".md"):
		return PathInfo{Visibility: VisibilityPrivate, Kind: "daily"}, true
	case strings.HasPrefix(rel, "memory/") && strings.HasSuffix(rel, ".md"):
		return PathInfo{Visibility: VisibilityPrivate, Kind: "daily"}, true
	default:
		return PathInfo{}, false
	}
}

func ResolvePathInfo(workspace, rawPath string) (string, string, PathInfo, error) {
	p := strings.TrimSpace(rawPath)
	if p == "" {
		return "", "", PathInfo{}, fmt.Errorf("%w: empty path", ErrPathDenied)
	}

	clean := filepath.Clean(filepath.FromSlash(p))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", "", PathInfo{}, fmt.Errorf("%w: outside workspace", ErrPathDenied)
	}

	rel := filepath.ToSlash(clean)
	info, allowed := describeMemoryPath(rel)
	if !allowed {
		return "", "", PathInfo{}, fmt.Errorf("%w: path not in whitelist", ErrPathDenied)
	}

	abs := filepath.Join(workspace, clean)
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return "", "", PathInfo{}, err
	}
	workspaceReal := workspaceAbs
	if resolvedWorkspace, err := filepath.EvalSymlinks(workspaceAbs); err == nil {
		workspaceReal = resolvedWorkspace
	}
	absPath, err := filepath.Abs(abs)
	if err != nil {
		return "", "", PathInfo{}, err
	}
	inside, err := isWithinWorkspace(workspaceAbs, absPath)
	if err != nil {
		return "", "", PathInfo{}, err
	}
	if !inside {
		return "", "", PathInfo{}, fmt.Errorf("%w: outside workspace", ErrPathDenied)
	}

	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		resolvedAbs, err := filepath.Abs(resolved)
		if err != nil {
			return "", "", PathInfo{}, err
		}
		inside, err := isWithinWorkspace(workspaceReal, resolvedAbs)
		if err != nil {
			return "", "", PathInfo{}, err
		}
		if !inside {
			return "", "", PathInfo{}, fmt.Errorf("%w: symlink escapes workspace", ErrPathDenied)
		}
		resolvedRel, err := filepath.Rel(workspaceReal, resolvedAbs)
		if err != nil {
			return "", "", PathInfo{}, err
		}
		resolvedRel = filepath.ToSlash(filepath.Clean(resolvedRel))
		resolvedInfo, resolvedAllowed := describeMemoryPath(resolvedRel)
		if !resolvedAllowed {
			return "", "", PathInfo{}, fmt.Errorf("%w: symlink target not in whitelist", ErrPathDenied)
		}
		absPath = resolvedAbs
		info = resolvedInfo
	} else if !os.IsNotExist(err) {
		return "", "", PathInfo{}, err
	}

	return rel, absPath, info, nil
}

// ResolvePath validates a memory path and returns normalized relative + absolute path.
func ResolvePath(workspace, rawPath string) (string, string, string, error) {
	rel, abs, info, err := ResolvePathInfo(workspace, rawPath)
	if err != nil {
		return "", "", "", err
	}
	return rel, abs, info.Visibility, nil
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
			rel, _, info, err := ResolvePathInfo(workspace, filepath.ToSlash(relPath))
			if err != nil {
				return nil
			}
			out = append(out, MemoryFile{Path: rel, Visibility: info.Visibility, Kind: info.Kind})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	if stat, err := os.Stat(filepath.Join(workspace, "MEMORY.md")); err == nil && !stat.IsDir() {
		if rel, _, info, err := ResolvePathInfo(workspace, "MEMORY.md"); err == nil {
			out = append(out, MemoryFile{Path: rel, Visibility: info.Visibility, Kind: info.Kind})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out, nil
}
