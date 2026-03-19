package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/messages"
)

func (m *modelState) View() string {
	switch m.mode {
	case modeSelector:
		return m.renderSelector()
	case modeNaming:
		return m.renderNaming()
	default:
		return m.renderChat()
	}
}

func (m *modelState) renderSelector() string {
	var b strings.Builder
	b.WriteString(messages.Chat.SelectorTitle)
	b.WriteString(messages.Chat.SelectorHelp)
	if m.selectorBusy {
		b.WriteString(messages.Chat.SelectorLoading)
	} else if len(m.sessions) == 0 {
		b.WriteString(messages.Chat.SelectorEmpty)
	} else {
		for i, item := range m.sessions {
			cursor := "  "
			if i == m.selectorIdx {
				cursor = "> "
			}
			b.WriteString(messages.Chat.SelectorItem(cursor, item.ConversationID, item.MessageCount, item.LastModel, item.LastActivityAt.Format(time.RFC3339)))
		}
	}
	b.WriteString("\n")
	b.WriteString(m.renderStatus())
	return b.String()
}

func (m *modelState) renderNaming() string {
	return messages.Chat.NamingView(m.nameInput.View(), m.renderStatus())
}

func (m *modelState) renderChat() string {
	header := messages.Chat.Header(nonEmpty(m.conversation, messages.Chat.ConversationMissing), nonEmpty(m.sessionKey, messages.Chat.SessionPending))
	help := messages.Chat.Help
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		strings.Repeat("─", max(10, m.width-1)),
		m.viewport.View(),
		strings.Repeat("─", max(10, m.width-1)),
		m.input.View(),
		help,
		m.renderStatus(),
	)
}

func (m *modelState) renderStatus() string {
	parts := []string{m.status}
	if m.lastError != "" {
		parts = append(parts, messages.Chat.StatusErrorPrefix+m.lastError)
	}
	return strings.Join(parts, "  |  ")
}

func renderMessages(items []chatMessage) string {
	if len(items) == 0 {
		return messages.Chat.NoMessages
	}
	var parts []string
	for _, msg := range items {
		var prefix string
		switch msg.Role {
		case "user":
			prefix = messages.Chat.MessagePrefixUser
		case "assistant":
			prefix = messages.Chat.MessagePrefixAssistant
		default:
			prefix = msg.Role
		}
		parts = append(parts, fmt.Sprintf("%s> %s", prefix, msg.Content))
	}
	return strings.Join(parts, "\n\n")
}

func defaultConversationID() string {
	now := time.Now().UTC()
	return fmt.Sprintf("cli_%s_%09d", now.Format("20060102T150405Z"), now.Nanosecond())
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
