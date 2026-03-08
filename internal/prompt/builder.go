package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

var bootstrapFiles = []string{"AGENTS.md", "IDENTITY.md", "USER.md"}

type Builder struct {
	workspace string

	mu           sync.RWMutex
	cachedStatic string
	cachedAt     time.Time
	existed      map[string]bool
	staticBuilds int
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
	if b.cachedStatic != "" && !b.bootstrapChangedLocked() {
		cached := b.cachedStatic
		b.mu.RUnlock()
		return cached
	}
	b.mu.RUnlock()

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cachedStatic != "" && !b.bootstrapChangedLocked() {
		return b.cachedStatic
	}

	parts := []string{
		b.identitySection(),
		b.projectContextSection(),
		b.availableSkillsSection(),
		b.memoryPolicySection(),
	}
	b.cachedStatic = strings.Join(parts, "\n\n---\n\n")
	b.cachedAt, b.existed = b.snapshotBootstrapState()
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
		content, ok := b.readBootstrapFile(name)
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
	return strings.TrimSpace(`## Available Skills

当前轮次未注入 skill 索引。`)
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

func (b *Builder) readBootstrapFile(name string) (string, bool) {
	path := filepath.Join(b.workspace, name)
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

func (b *Builder) snapshotBootstrapState() (time.Time, map[string]bool) {
	var maxMtime time.Time
	existed := make(map[string]bool, len(bootstrapFiles))
	for _, name := range bootstrapFiles {
		path := filepath.Join(b.workspace, name)
		info, err := os.Stat(path)
		existed[path] = err == nil
		if err == nil && info.ModTime().After(maxMtime) {
			maxMtime = info.ModTime()
		}
	}
	if maxMtime.IsZero() {
		maxMtime = time.Unix(1, 0).UTC()
	}
	return maxMtime, existed
}

func (b *Builder) bootstrapChangedLocked() bool {
	for _, name := range bootstrapFiles {
		path := filepath.Join(b.workspace, name)
		info, err := os.Stat(path)
		existsNow := err == nil
		if b.existed[path] != existsNow {
			return true
		}
		if err == nil && info.ModTime().After(b.cachedAt) {
			return true
		}
	}
	return false
}
