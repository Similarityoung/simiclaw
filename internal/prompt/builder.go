package prompt

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/internal/contextfile"
	"github.com/similarityyoung/simiclaw/internal/memory"
	"github.com/similarityyoung/simiclaw/pkg/model"
	promptpkg "github.com/similarityyoung/simiclaw/pkg/prompt"
)

var workspaceContextFiles = []string{"SOUL.md", "IDENTITY.md", "USER.md", "AGENTS.md", "TOOLS.md"}

const (
	bootstrapContextFile = "BOOTSTRAP.md"
	heartbeatContextFile = "HEARTBEAT.md"
)

type Builder struct {
	workspace string

	mu           sync.RWMutex
	cachedStatic map[string]staticCacheEntry
	staticBuilds int
}

type staticCacheEntry struct {
	content      string
	fingerprints map[string]string
}

type staticVariant struct {
	memoryMode       string
	includeHeartbeat bool
}

func (v staticVariant) key() string {
	if v.includeHeartbeat {
		return v.memoryMode + "|heartbeat"
	}
	return v.memoryMode + "|normal"
}

type RunContext struct {
	Now          time.Time
	Conversation model.Conversation
	SessionKey   string
	SessionID    string
	PayloadType  string
}

type BuildInput struct {
	Context RunContext
}

type SkillSummary struct {
	Name        string
	Description string
	Path        string
}

type textEntry struct {
	DisplayPath  string
	ResolvedPath string
	Content      string
}

func NewBuilder(workspace string) *Builder {
	return &Builder{workspace: workspace, cachedStatic: map[string]staticCacheEntry{}}
}

func (b *Builder) Build(input BuildInput) string {
	now := input.Context.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}

	parts := []string{
		b.buildStaticPrefix(buildStaticVariant(input.Context)),
		b.currentRunContextSection(input.Context, now),
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func buildStaticVariant(ctx RunContext) staticVariant {
	return staticVariant{
		memoryMode:       buildMemoryMode(ctx.Conversation.ChannelType),
		includeHeartbeat: strings.EqualFold(strings.TrimSpace(ctx.PayloadType), "cron_fire"),
	}
}

func (b *Builder) buildStaticPrefix(variant staticVariant) string {
	key := variant.key()
	snapshot := b.snapshotStaticState(variant)

	b.mu.RLock()
	if entry, ok := b.cachedStatic[key]; ok && equalStringMap(entry.fingerprints, snapshot) {
		cached := entry.content
		b.mu.RUnlock()
		return cached
	}
	b.mu.RUnlock()

	content := b.buildStaticContent(variant)
	snapshot = b.snapshotStaticState(variant)

	b.mu.Lock()
	defer b.mu.Unlock()
	if entry, ok := b.cachedStatic[key]; ok {
		latest := b.snapshotStaticState(variant)
		if equalStringMap(entry.fingerprints, latest) {
			return entry.content
		}
		snapshot = latest
	}
	b.cachedStatic[key] = staticCacheEntry{content: content, fingerprints: snapshot}
	b.staticBuilds++
	return content
}

func (b *Builder) buildStaticContent(variant staticVariant) string {
	parts := []string{
		b.identitySection(),
		b.toolContractSection(),
		b.memoryPolicySection(variant.memoryMode),
		b.workspaceContextSection(),
		b.availableSkillsSection(),
	}
	if variant.includeHeartbeat {
		parts = append(parts, b.heartbeatPolicySection())
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func (b *Builder) identitySection() string {
	workspacePath, err := filepath.Abs(b.workspace)
	if err != nil {
		workspacePath = b.workspace
	}
	return promptpkg.Render(promptpkg.SystemText.IdentityRuntime, map[string]string{"workspace_path": workspacePath})
}

func (b *Builder) toolContractSection() string {
	return promptpkg.SystemText.ToolContract
}

func (b *Builder) memoryPolicySection(memoryMode string) string {
	parts := []string{promptpkg.SystemText.MemoryPolicy}
	blocks := b.curatedMemoryBlocks(memoryMode)
	if len(blocks) == 0 {
		parts = append(parts, "### Injected Curated Memory\n\nNo curated memory is injected for this run.")
		return strings.Join(parts, "\n\n")
	}
	parts = append(parts, "### Injected Curated Memory")
	parts = append(parts, blocks...)
	return strings.Join(parts, "\n\n")
}

func (b *Builder) workspaceContextSection() string {
	parts := []string{"## Workspace Instructions & Context"}
	loaded := 0
	for _, name := range append(append([]string{}, workspaceContextFiles...), bootstrapContextFile) {
		entry, ok := b.readContextText(name)
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("### %s\n\n%s", entry.DisplayPath, entry.Content))
		loaded++
	}
	if loaded == 0 {
		parts = append(parts, "No extra workspace context files are injected for this run.")
	}
	return strings.Join(parts, "\n\n")
}

func (b *Builder) availableSkillsSection() string {
	skills := b.loadSkillSummaries()
	parts := []string{"## Available Skills"}
	if len(skills) == 0 {
		parts = append(parts, "No skills were found in the current workspace.")
		return strings.Join(parts, "\n\n")
	}
	parts = append(parts, "To read a skill body, use context_get on `skills/<name>/SKILL.md` first.")
	for _, skill := range skills {
		parts = append(parts, fmt.Sprintf("- %s — %s (%s)", skill.Name, skill.Description, skill.Path))
	}
	return strings.Join(parts, "\n\n")
}

func (b *Builder) heartbeatPolicySection() string {
	parts := []string{promptpkg.SystemText.HeartbeatPolicy}
	entry, ok := b.readContextText(heartbeatContextFile)
	if !ok {
		parts = append(parts, "### HEARTBEAT.md\n\nThe current workspace does not provide HEARTBEAT.md. Follow the conservative default policy.")
		return strings.Join(parts, "\n\n")
	}
	parts = append(parts, fmt.Sprintf("### %s\n\n%s", entry.DisplayPath, entry.Content))
	return strings.Join(parts, "\n\n")
}

func (b *Builder) currentRunContextSection(ctx RunContext, now time.Time) string {
	return strings.TrimSpace(fmt.Sprintf(`## Current Run Context

- current_time_utc: %s
- conversation_id: %s
- thread_id: %s
- channel_type: %s
- participant_id: %s
- session_key: %s
- session_id: %s
- payload_type: %s`,
		now.Format(time.RFC3339),
		promptLiteral(ctx.Conversation.ConversationID),
		promptLiteral(ctx.Conversation.ThreadID),
		promptLiteral(ctx.Conversation.ChannelType),
		promptLiteral(ctx.Conversation.ParticipantID),
		promptLiteral(ctx.SessionKey),
		promptLiteral(ctx.SessionID),
		promptLiteral(ctx.PayloadType)))
}

func promptLiteral(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return strconv.Quote(value)
}

func (b *Builder) readContextText(rel string) (textEntry, bool) {
	normalizedRel, absPath, err := contextfile.ResolvePath(b.workspace, rel)
	if err != nil {
		return textEntry{}, false
	}
	content, ok := b.readFileText(absPath)
	if !ok {
		return textEntry{}, false
	}
	return textEntry{DisplayPath: normalizedRel, ResolvedPath: resolvedPath(absPath), Content: content}, true
}

func (b *Builder) readMemoryText(rel, memoryMode string) (textEntry, bool) {
	normalizedRel, absPath, visibility, err := memory.ResolvePath(b.workspace, rel)
	if err != nil {
		return textEntry{}, false
	}
	if !canAccessMemoryVisibility(memoryMode, visibility) {
		return textEntry{}, false
	}
	content, ok := b.readFileText(absPath)
	if !ok {
		return textEntry{}, false
	}
	return textEntry{DisplayPath: normalizedRel, ResolvedPath: resolvedPath(absPath), Content: content}, true
}

func (b *Builder) readFileText(absPath string) (string, bool) {
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

func (b *Builder) loadSkillSummaries() []SkillSummary {
	root := filepath.Join(b.workspace, "skills")
	entries := make([]SkillSummary, 0, 8)
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		relPath, err := filepath.Rel(b.workspace, path)
		if err != nil {
			return nil
		}
		entry, ok := b.readContextText(filepath.ToSlash(relPath))
		if !ok {
			return nil
		}
		skill, ok := parseSkillSummary(b.workspace, filepath.Join(b.workspace, filepath.FromSlash(entry.DisplayPath)), entry.Content)
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

func (b *Builder) snapshotStaticState(variant staticVariant) map[string]string {
	state := map[string]string{}
	for _, name := range workspaceContextFiles {
		state["ctx:"+name] = b.snapshotContextFileFingerprint(name)
	}
	state["ctx:"+bootstrapContextFile] = b.snapshotContextFileFingerprint(bootstrapContextFile)
	if variant.includeHeartbeat {
		state["ctx:"+heartbeatContextFile] = b.snapshotContextFileFingerprint(heartbeatContextFile)
	}
	for rel, fingerprint := range b.snapshotSkillState() {
		state["skill:"+rel] = fingerprint
	}
	for rel, fingerprint := range b.snapshotCuratedState(variant.memoryMode) {
		state["memory:"+rel] = fingerprint
	}
	return state
}

func (b *Builder) snapshotSkillState() map[string]string {
	state := map[string]string{}
	root := filepath.Join(b.workspace, "skills")
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		rel, err := filepath.Rel(b.workspace, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		state[rel] = b.snapshotContextFileFingerprint(rel)
		return nil
	})
	return state
}

func (b *Builder) snapshotCuratedState(memoryMode string) map[string]string {
	state := map[string]string{}
	candidates := []string{filepath.ToSlash(filepath.Join("memory", "public", "MEMORY.md")), "MEMORY.md"}
	if memoryMode == "public_private" {
		candidates = append(candidates, filepath.ToSlash(filepath.Join("memory", "private", "MEMORY.md")))
	}
	for _, rel := range candidates {
		state[rel] = b.snapshotMemoryFileFingerprint(rel, memoryMode)
	}
	return state
}

func (b *Builder) snapshotContextFileFingerprint(rel string) string {
	absCandidate := filepath.Join(b.workspace, filepath.FromSlash(rel))
	info, err := os.Lstat(absCandidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "stat_error"
	}
	normalizedRel, absPath, err := contextfile.ResolvePath(b.workspace, rel)
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

func (b *Builder) snapshotMemoryFileFingerprint(rel, memoryMode string) string {
	absCandidate := filepath.Join(b.workspace, filepath.FromSlash(rel))
	info, err := os.Lstat(absCandidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "stat_error"
	}
	normalizedRel, absPath, visibility, err := memory.ResolvePath(b.workspace, rel)
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

func fileMarker(path string, info os.FileInfo) string {
	marker := info.Mode().String()
	if info.Mode()&os.ModeSymlink != 0 {
		if target, err := os.Readlink(path); err == nil {
			marker += ":" + target
		}
	}
	return marker
}

func (b *Builder) curatedMemoryBlocks(memoryMode string) []string {
	blocks := make([]string, 0, 3)
	seen := map[string]bool{}
	for _, rel := range []string{filepath.ToSlash(filepath.Join("memory", "public", "MEMORY.md")), "MEMORY.md"} {
		entry, ok := b.readMemoryText(rel, memoryMode)
		if !ok || seen[entry.ResolvedPath] {
			continue
		}
		seen[entry.ResolvedPath] = true
		blocks = append(blocks, fmt.Sprintf("#### %s\n\n%s", entry.DisplayPath, entry.Content))
	}
	if memoryMode == "public_private" {
		entry, ok := b.readMemoryText(filepath.ToSlash(filepath.Join("memory", "private", "MEMORY.md")), memoryMode)
		if ok && !seen[entry.ResolvedPath] {
			blocks = append(blocks, fmt.Sprintf("#### %s\n\n%s", entry.DisplayPath, entry.Content))
		}
	}
	return blocks
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

func equalStringMap(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		rightValue, ok := right[key]
		if !ok {
			return false
		}
		if leftValue != rightValue {
			return false
		}
	}
	return true
}
