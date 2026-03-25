package runner

import (
	"strings"

	runnermodel "github.com/similarityyoung/simiclaw/internal/runner/model"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
)

func historyToChatMessages(history []runnermodel.HistoryMessage) []kernel.ModelMessage {
	out := make([]kernel.ModelMessage, 0, len(history))
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
			out = append(out, kernel.ModelMessage{
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
			out = append(out, kernel.ModelMessage{
				Role:       msg.Role,
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
			})
			delete(pendingToolCalls, msg.ToolCallID)
		default:
			out = append(out, kernel.ModelMessage{Role: msg.Role, Content: msg.Content, ToolCallID: msg.ToolCallID})
		}
	}
	return out
}

func shouldSkipPromptHistoryPayloadType(payloadType any) bool {
	value, _ := payloadType.(string)
	return value == "cron_fire" || value == payloadTypeNewSession
}
