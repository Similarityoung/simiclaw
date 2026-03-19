package prompt

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/similarityyoung/simiclaw/internal/memory"
	"github.com/similarityyoung/simiclaw/internal/workspacefile"
)

type promptFingerprinter struct {
	workspace string
}

func (f promptFingerprinter) snapshotStaticState(variant staticVariant) map[string]string {
	state := map[string]string{}
	for _, name := range workspaceContextFiles {
		state["ctx:"+name] = f.snapshotContextFileFingerprint(name)
	}
	state["ctx:"+bootstrapContextFile] = f.snapshotContextFileFingerprint(bootstrapContextFile)
	if variant.includeHeartbeat {
		state["ctx:"+heartbeatContextFile] = f.snapshotContextFileFingerprint(heartbeatContextFile)
	}
	for rel, fingerprint := range f.snapshotSkillState() {
		state["skill:"+rel] = fingerprint
	}
	for rel, fingerprint := range f.snapshotCuratedState(variant.memoryMode) {
		state["memory:"+rel] = fingerprint
	}
	return state
}

func (f promptFingerprinter) snapshotSkillState() map[string]string {
	state := map[string]string{}
	root := filepath.Join(f.workspace, "skills")
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		rel, err := filepath.Rel(f.workspace, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		state[rel] = f.snapshotContextFileFingerprint(rel)
		return nil
	})
	return state
}

func (f promptFingerprinter) snapshotCuratedState(memoryMode string) map[string]string {
	state := map[string]string{}
	candidates := []string{filepath.ToSlash(filepath.Join("memory", "public", "MEMORY.md")), "MEMORY.md"}
	if memoryMode == "public_private" {
		candidates = append(candidates, filepath.ToSlash(filepath.Join("memory", "private", "MEMORY.md")))
	}
	for _, rel := range candidates {
		state[rel] = f.snapshotMemoryFileFingerprint(rel, memoryMode)
	}
	return state
}

func (f promptFingerprinter) snapshotContextFileFingerprint(rel string) string {
	absCandidate := filepath.Join(f.workspace, filepath.FromSlash(rel))
	info, err := os.Lstat(absCandidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "stat_error"
	}
	normalizedRel, absPath, err := workspacefile.ResolveContextPath(f.workspace, rel)
	if err != nil {
		return "denied:" + fileMarker(absCandidate, info)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "read_error"
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "empty:" + normalizedRel + ":" + resolvedPath(absPath)
	}
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("ok:%s:%s:%x", normalizedRel, resolvedPath(absPath), sum)
}

func (f promptFingerprinter) snapshotMemoryFileFingerprint(rel, memoryMode string) string {
	absCandidate := filepath.Join(f.workspace, filepath.FromSlash(rel))
	info, err := os.Lstat(absCandidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "stat_error"
	}
	normalizedRel, absPath, visibility, err := memory.ResolvePath(f.workspace, rel)
	if err != nil {
		return "denied:" + fileMarker(absCandidate, info)
	}
	if !canAccessMemoryVisibility(memoryMode, visibility) {
		return "denied_visibility:" + visibility
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "read_error"
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "empty:" + normalizedRel + ":" + resolvedPath(absPath)
	}
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("ok:%s:%s:%x", normalizedRel, resolvedPath(absPath), sum)
}

func resolvedPath(absPath string) string {
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		if resolvedAbs, err := filepath.Abs(resolved); err == nil {
			return resolvedAbs
		}
	}
	if normalizedAbs, err := filepath.Abs(absPath); err == nil {
		return normalizedAbs
	}
	return absPath
}

func fileMarker(path string, info os.FileInfo) string {
	marker := info.Mode().String()
	if info.Mode()&os.ModeSymlink != 0 {
		if target, err := os.Readlink(path); err == nil {
			marker += ":" + target
		}
	}
	return marker
}
