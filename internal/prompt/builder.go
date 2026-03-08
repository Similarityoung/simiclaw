package prompt

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

var bootstrapFiles = []string{"AGENTS.md", "IDENTITY.md", "USER.md"}

type Builder struct {
	workspace string

	mu                sync.RWMutex
	cachedStatic      string
	bootstrapAtCache  map[string]time.Time
	skillFilesAtCache map[string]time.Time
	skillDirsAtCache  map[string]time.Time
	staticBuilds      int
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

func NewBuilder(workspace string) *Builder {
	return &Builder{workspace: workspace}
}

func (b *Builder) Build(input BuildInput) string {
	now := input.Context.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}

	parts := []string{
		b.buildStaticPrefix(),
		b.currentRunContextSection(input.Context, now),
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func (b *Builder) buildStaticPrefix() string {
	b.mu.RLock()
	if b.cachedStatic != "" && !b.staticSourcesChangedLocked() {
		cached := b.cachedStatic
		b.mu.RUnlock()
		return cached
	}
	b.mu.RUnlock()

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cachedStatic != "" && !b.staticSourcesChangedLocked() {
		return b.cachedStatic
	}

	parts := []string{
		b.identitySection(),
		b.projectContextSection(),
		b.availableSkillsSection(),
		b.memoryPolicySection(),
	}
	b.cachedStatic = strings.Join(parts, "\n\n---\n\n")
	b.bootstrapAtCache = b.snapshotBootstrapState()
	b.skillFilesAtCache, b.skillDirsAtCache = b.snapshotSkillState()
	b.staticBuilds++
	return b.cachedStatic
}

func (b *Builder) identitySection() string {
	workspacePath, err := filepath.Abs(b.workspace)
	if err != nil {
		workspacePath = b.workspace
	}
	return strings.TrimSpace(fmt.Sprintf(`## Identity & Runtime Rules

你是 SimiClaw，一个运行在本地工作区内的 Go Agent Runtime 助手。

- 当前工作区：%s
- 回答默认跟随用户语言；若用户未指定，则优先使用用户消息的语言。
- 涉及执行动作、读取记忆或使用扩展能力时，优先使用工具，不要假装已经执行。
- 遵守工作区内显式规则文件；显式用户指令优先于近似上下文。`, workspacePath))
}

func (b *Builder) projectContextSection() string {
	parts := make([]string, 0, len(bootstrapFiles)+1)
	parts = append(parts, "## Project Context")
	loaded := 0
	for _, name := range bootstrapFiles {
		content, ok := b.readWorkspaceText(name)
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("### %s\n\n%s", name, content))
		loaded++
	}
	if loaded == 0 {
		parts = append(parts, "当前轮次未注入额外的工作区 bootstrap 内容。")
	}
	return strings.Join(parts, "\n\n")
}

func (b *Builder) availableSkillsSection() string {
	skills := b.loadSkillSummaries()
	parts := []string{
		"## Available Skills",
	}
	if len(skills) == 0 {
		parts = append(parts, "当前工作区未发现可用 skill。")
		return strings.Join(parts, "\n\n")
	}
	parts = append(parts, "需要 skill 正文时，先使用 context_get 读取对应的 `skills/<name>/SKILL.md`。")
	for _, skill := range skills {
		parts = append(parts, fmt.Sprintf("- %s — %s (%s)", skill.Name, skill.Description, skill.Path))
	}
	return strings.Join(parts, "\n\n")
}

func (b *Builder) memoryPolicySection() string {
	return strings.TrimSpace(`## Memory Policy

- 记忆应通过显式 recall 获取，不要声称自己“天然记得”工作区事实。
- 当问题可能依赖历史偏好、长期事实或日常记录时，优先使用 memory_search，再按需使用 memory_get。
- 近似上下文仅供参考；若与显式指令冲突，以显式指令为准。`)
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
		blankFallback(ctx.Conversation.ConversationID),
		blankFallback(ctx.Conversation.ThreadID),
		blankFallback(ctx.Conversation.ChannelType),
		blankFallback(ctx.Conversation.ParticipantID),
		blankFallback(ctx.SessionKey),
		blankFallback(ctx.SessionID),
		blankFallback(ctx.PayloadType)))
}

func blankFallback(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func (b *Builder) readWorkspaceText(rel string) (string, bool) {
	path := filepath.Join(b.workspace, filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", false
	}
	return content, true
}

func (b *Builder) loadSkillSummaries() []SkillSummary {
	root := filepath.Join(b.workspace, "skills")
	entries := make([]SkillSummary, 0, 8)
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		skill, ok := parseSkillSummary(b.workspace, path, string(data))
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
		description = "无描述"
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

func (b *Builder) snapshotBootstrapState() map[string]time.Time {
	state := make(map[string]time.Time, len(bootstrapFiles))
	for _, name := range bootstrapFiles {
		path := filepath.Join(b.workspace, name)
		if info, err := os.Stat(path); err == nil {
			state[path] = info.ModTime()
			continue
		}
		state[path] = time.Time{}
	}
	return state
}

func (b *Builder) snapshotSkillState() (map[string]time.Time, map[string]time.Time) {
	files := map[string]time.Time{}
	dirs := map[string]time.Time{}
	root := filepath.Join(b.workspace, "skills")
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil
		}
		if d.IsDir() {
			dirs[path] = info.ModTime()
			return nil
		}
		if d.Name() == "SKILL.md" {
			files[path] = info.ModTime()
		}
		return nil
	})
	return files, dirs
}

func (b *Builder) staticSourcesChangedLocked() bool {
	if !equalTimeMap(b.bootstrapAtCache, b.snapshotBootstrapState()) {
		return true
	}
	files, dirs := b.snapshotSkillState()
	if !equalTimeMap(b.skillFilesAtCache, files) {
		return true
	}
	if !equalTimeMap(b.skillDirsAtCache, dirs) {
		return true
	}
	return false
}

func equalTimeMap(left, right map[string]time.Time) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		rightValue, ok := right[key]
		if !ok {
			return false
		}
		if !leftValue.Equal(rightValue) {
			return false
		}
	}
	return true
}
