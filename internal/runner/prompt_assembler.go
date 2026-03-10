package runner

import (
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/internal/prompt"
	"github.com/similarityyoung/simiclaw/internal/provider"
	runnermodel "github.com/similarityyoung/simiclaw/internal/runner/model"
	"github.com/similarityyoung/simiclaw/internal/tools"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type llmPromptAssembly struct {
	chatMessages []provider.ChatMessage
	toolDefs     []provider.ToolDefinition
}

type llmPromptAssembler struct {
	prompts  *prompt.Builder
	registry *tools.Registry
}

func (a llmPromptAssembler) Assemble(event model.InternalEvent, now time.Time, history []runnermodel.HistoryMessage, allowedTools map[string]struct{}) llmPromptAssembly {
	userText := strings.TrimSpace(event.Payload.Text)
	systemPrompt := a.prompts.Build(prompt.BuildInput{Context: prompt.RunContext{
		Now:          now,
		Conversation: event.Conversation,
		SessionKey:   event.SessionKey,
		SessionID:    event.ActiveSessionID,
		PayloadType:  event.Payload.Type,
	}})

	chatMessages := make([]provider.ChatMessage, 0, len(history)+2)
	chatMessages = append(chatMessages, provider.ChatMessage{Role: "system", Content: systemPrompt})
	chatMessages = append(chatMessages, historyToChatMessages(history)...)
	chatMessages = append(chatMessages, provider.ChatMessage{Role: "user", Content: userText})

	toolDefs := make([]provider.ToolDefinition, 0)
	for _, def := range a.registry.Definitions() {
		if !toolAllowed(def.Schema.Name, allowedTools) {
			continue
		}
		toolDefs = append(toolDefs, provider.ToolDefinition{
			Name:        def.Schema.Name,
			Description: def.Schema.Description,
			Parameters:  def.Schema.Parameters,
		})
	}

	return llmPromptAssembly{
		chatMessages: chatMessages,
		toolDefs:     toolDefs,
	}
}
