package runner

import (
	"strings"

	"github.com/similarityyoung/simiclaw/internal/provider"
	"github.com/similarityyoung/simiclaw/internal/store"
)

func historyToChatMessages(history []store.HistoryMessage) []provider.ChatMessage {
	out := make([]provider.ChatMessage, 0, len(history))
	pendingToolCalls := map[string]bool{}
	for _, msg := range history {
		if shouldSkipPromptHistoryPayloadType(msg.Meta[payloadTypeMetaKey]) {
			continue
		}
		switch msg.Role {
		case "assistant":
			if strings.TrimSpace(msg.Content) == "" && len(msg.ToolCalls) == 0 {
				continue
			}
			out = append(out, provider.ChatMessage{
				Role:      msg.Role,
				Content:   msg.Content,
				ToolCalls: cloneToolCalls(msg.ToolCalls),
			})
			for _, call := range msg.ToolCalls {
				if strings.TrimSpace(call.ToolCallID) == "" {
					continue
				}
				pendingToolCalls[call.ToolCallID] = true
			}
		case "tool":
			if strings.TrimSpace(msg.ToolCallID) == "" || !pendingToolCalls[msg.ToolCallID] {
				continue
			}
			out = append(out, provider.ChatMessage{
				Role:       msg.Role,
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
			})
			delete(pendingToolCalls, msg.ToolCallID)
		default:
			out = append(out, provider.ChatMessage{Role: msg.Role, Content: msg.Content, ToolCallID: msg.ToolCallID})
		}
	}
	return out
}

func shouldSkipPromptHistoryPayloadType(payloadType any) bool {
	value, _ := payloadType.(string)
	return value == "cron_fire" || value == payloadTypeNewSession
}
