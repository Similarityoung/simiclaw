package prompt

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type Builder struct {
	workspace string
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
		b.identitySection(),
		b.projectContextSection(),
		b.availableSkillsSection(),
		b.memoryPolicySection(),
		b.currentRunContextSection(input.Context, now),
	}
	return strings.Join(parts, "\n\n---\n\n")
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
	return strings.TrimSpace(`## Project Context

当前轮次未注入额外的工作区 bootstrap 内容。`)
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
