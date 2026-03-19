package prompt

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type promptRenderer struct{}

func (promptRenderer) renderStatic(data staticPromptData) string {
	parts := []string{
		renderIdentitySection(data.workspacePath),
		renderToolContractSection(),
		renderMemoryPolicySection(data.memoryBlocks),
		renderWorkspaceContextSection(data.workspaceContext),
		renderAvailableSkillsSection(data.skills),
	}
	if data.includeHeartbeat {
		parts = append(parts, renderHeartbeatPolicySection(data.heartbeat, data.hasHeartbeat))
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func renderIdentitySection(workspacePath string) string {
	return renderSystemTemplate(systemText.IdentityRuntime, map[string]string{"workspace_path": workspacePath})
}

func renderToolContractSection() string {
	return systemText.ToolContract
}

func renderMemoryPolicySection(blocks []textEntry) string {
	parts := []string{systemText.MemoryPolicy}
	if len(blocks) == 0 {
		parts = append(parts, "### Injected Curated Memory\n\nNo curated memory is injected for this run.")
		return strings.Join(parts, "\n\n")
	}
	parts = append(parts, "### Injected Curated Memory")
	for _, block := range blocks {
		parts = append(parts, fmt.Sprintf("#### %s\n\n%s", block.DisplayPath, block.Content))
	}
	return strings.Join(parts, "\n\n")
}

func renderWorkspaceContextSection(entries []textEntry) string {
	parts := []string{"## Workspace Instructions & Context"}
	if len(entries) == 0 {
		parts = append(parts, "No extra workspace context files are injected for this run.")
		return strings.Join(parts, "\n\n")
	}
	for _, entry := range entries {
		parts = append(parts, fmt.Sprintf("### %s\n\n%s", entry.DisplayPath, entry.Content))
	}
	return strings.Join(parts, "\n\n")
}

func renderAvailableSkillsSection(skills []SkillSummary) string {
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

func renderHeartbeatPolicySection(entry textEntry, ok bool) string {
	parts := []string{systemText.HeartbeatPolicy}
	if !ok {
		parts = append(parts, "### HEARTBEAT.md\n\nThe current workspace does not provide HEARTBEAT.md. Follow the conservative default policy.")
		return strings.Join(parts, "\n\n")
	}
	parts = append(parts, fmt.Sprintf("### %s\n\n%s", entry.DisplayPath, entry.Content))
	return strings.Join(parts, "\n\n")
}

func (promptRenderer) renderCurrentRunContext(ctx RunContext, now time.Time) string {
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
