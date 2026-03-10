package prompt

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/similarityyoung/simiclaw/internal/contextfile"
	"github.com/similarityyoung/simiclaw/internal/memory"
)

type promptLoader struct {
	workspace string
}

func (l promptLoader) loadStaticData(variant staticVariant) staticPromptData {
	data := staticPromptData{
		workspacePath:    l.workspacePath(),
		memoryBlocks:     l.loadCuratedMemoryBlocks(variant.memoryMode),
		workspaceContext: l.loadWorkspaceContextEntries(),
		skills:           l.loadSkillSummaries(),
		includeHeartbeat: variant.includeHeartbeat,
	}
	if variant.includeHeartbeat {
		data.heartbeat, data.hasHeartbeat = l.readContextText(heartbeatContextFile)
	}
	return data
}

func (l promptLoader) workspacePath() string {
	workspacePath, err := filepath.Abs(l.workspace)
	if err != nil {
		return l.workspace
	}
	return workspacePath
}

func (l promptLoader) loadWorkspaceContextEntries() []textEntry {
	names := append(append([]string{}, workspaceContextFiles...), bootstrapContextFile)
	entries := make([]textEntry, 0, len(names))
	for _, name := range names {
		entry, ok := l.readContextText(name)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func (l promptLoader) loadCuratedMemoryBlocks(memoryMode string) []textEntry {
	blocks := make([]textEntry, 0, 3)
	seen := map[string]bool{}
	for _, rel := range []string{filepath.ToSlash(filepath.Join("memory", "public", "MEMORY.md")), "MEMORY.md"} {
		entry, ok := l.readMemoryText(rel, memoryMode)
		if !ok || seen[entry.ResolvedPath] {
			continue
		}
		seen[entry.ResolvedPath] = true
		blocks = append(blocks, entry)
	}
	if memoryMode == "public_private" {
		entry, ok := l.readMemoryText(filepath.ToSlash(filepath.Join("memory", "private", "MEMORY.md")), memoryMode)
		if ok && !seen[entry.ResolvedPath] {
			blocks = append(blocks, entry)
		}
	}
	return blocks
}

func (l promptLoader) readContextText(rel string) (textEntry, bool) {
	normalizedRel, absPath, err := contextfile.ResolvePath(l.workspace, rel)
	if err != nil {
		return textEntry{}, false
	}
	content, ok := l.readFileText(absPath)
	if !ok {
		return textEntry{}, false
	}
	return textEntry{DisplayPath: normalizedRel, ResolvedPath: resolvedPath(absPath), Content: content}, true
}

func (l promptLoader) readMemoryText(rel, memoryMode string) (textEntry, bool) {
	normalizedRel, absPath, visibility, err := memory.ResolvePath(l.workspace, rel)
	if err != nil {
		return textEntry{}, false
	}
	if !canAccessMemoryVisibility(memoryMode, visibility) {
		return textEntry{}, false
	}
	content, ok := l.readFileText(absPath)
	if !ok {
		return textEntry{}, false
	}
	return textEntry{DisplayPath: normalizedRel, ResolvedPath: resolvedPath(absPath), Content: content}, true
}

func (l promptLoader) readFileText(absPath string) (string, bool) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", false
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", false
	}
	return content, true
}

func (l promptLoader) loadSkillSummaries() []SkillSummary {
	root := filepath.Join(l.workspace, "skills")
	entries := make([]SkillSummary, 0, 8)
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		relPath, err := filepath.Rel(l.workspace, path)
		if err != nil {
			return nil
		}
		entry, ok := l.readContextText(filepath.ToSlash(relPath))
		if !ok {
			return nil
		}
		skill, ok := parseSkillSummary(l.workspace, filepath.Join(l.workspace, filepath.FromSlash(entry.DisplayPath)), entry.Content)
		if !ok {
			log.Printf("prompt: skip invalid skill file %s", path)
			return nil
		}
		entries = append(entries, skill)
		return nil
	})
	sort.Slice(entries, func(i, j int) bool {
		left := strings.ToLower(entries[i].Name)
		right := strings.ToLower(entries[j].Name)
		if left == right {
			return entries[i].Path < entries[j].Path
		}
		return left < right
	})
	return entries
}

func (l promptLoader) snapshotStaticState(variant staticVariant) map[string]string {
	state := map[string]string{}
	for _, name := range workspaceContextFiles {
		state["ctx:"+name] = l.snapshotContextFileFingerprint(name)
	}
	state["ctx:"+bootstrapContextFile] = l.snapshotContextFileFingerprint(bootstrapContextFile)
	if variant.includeHeartbeat {
		state["ctx:"+heartbeatContextFile] = l.snapshotContextFileFingerprint(heartbeatContextFile)
	}
	for rel, fingerprint := range l.snapshotSkillState() {
		state["skill:"+rel] = fingerprint
	}
	for rel, fingerprint := range l.snapshotCuratedState(variant.memoryMode) {
		state["memory:"+rel] = fingerprint
	}
	return state
}

func (l promptLoader) snapshotSkillState() map[string]string {
	state := map[string]string{}
	root := filepath.Join(l.workspace, "skills")
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		rel, err := filepath.Rel(l.workspace, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		state[rel] = l.snapshotContextFileFingerprint(rel)
		return nil
	})
	return state
}

func (l promptLoader) snapshotCuratedState(memoryMode string) map[string]string {
	state := map[string]string{}
	candidates := []string{filepath.ToSlash(filepath.Join("memory", "public", "MEMORY.md")), "MEMORY.md"}
	if memoryMode == "public_private" {
		candidates = append(candidates, filepath.ToSlash(filepath.Join("memory", "private", "MEMORY.md")))
	}
	for _, rel := range candidates {
		state[rel] = l.snapshotMemoryFileFingerprint(rel, memoryMode)
	}
	return state
}

func (l promptLoader) snapshotContextFileFingerprint(rel string) string {
	absCandidate := filepath.Join(l.workspace, filepath.FromSlash(rel))
	info, err := os.Lstat(absCandidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "stat_error"
	}
	normalizedRel, absPath, err := contextfile.ResolvePath(l.workspace, rel)
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

func (l promptLoader) snapshotMemoryFileFingerprint(rel, memoryMode string) string {
	absCandidate := filepath.Join(l.workspace, filepath.FromSlash(rel))
	info, err := os.Lstat(absCandidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "stat_error"
	}
	normalizedRel, absPath, visibility, err := memory.ResolvePath(l.workspace, rel)
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

func parseSkillSummary(workspace, absPath, raw string) (SkillSummary, bool) {
	rel, err := filepath.Rel(workspace, absPath)
	if err != nil {
		return SkillSummary{}, false
	}
	rel = filepath.ToSlash(rel)
	name := filepath.Base(filepath.Dir(absPath))
	description := ""
	body := strings.TrimSpace(raw)
	if body == "" {
		return SkillSummary{}, false
	}
	if strings.HasPrefix(body, "---\n") {
		rest := body[len("---\n"):]
		idx := strings.Index(rest, "\n---\n")
		if idx < 0 {
			return SkillSummary{}, false
		}
		meta, ok := parseFrontmatter(rest[:idx])
		if !ok {
			return SkillSummary{}, false
		}
		body = strings.TrimSpace(rest[idx+len("\n---\n"):])
		if v := meta["name"]; v != "" {
			name = v
		}
		if v := meta["description"]; v != "" {
			description = v
		}
	}
	if description == "" {
		description = summarizeMarkdown(body)
	}
	if description == "" {
		description = "No description"
	}
	return SkillSummary{Name: name, Description: description, Path: rel}, true
}

func parseFrontmatter(raw string) (map[string]string, bool) {
	meta := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, false
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			return nil, false
		}
		meta[key] = value
	}
	return meta, true
}

func summarizeMarkdown(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "#-* ")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		runes := []rune(line)
		if len(runes) > 120 {
			return string(runes[:120]) + "..."
		}
		return line
	}
	return ""
}

func buildMemoryMode(channelType string) string {
	if strings.EqualFold(strings.TrimSpace(channelType), "dm") {
		return "public_private"
	}
	return "public_only"
}

func canAccessMemoryVisibility(memoryMode, visibility string) bool {
	if strings.EqualFold(strings.TrimSpace(visibility), memory.VisibilityPublic) {
		return true
	}
	return memoryMode == "public_private" && strings.EqualFold(strings.TrimSpace(visibility), memory.VisibilityPrivate)
}
