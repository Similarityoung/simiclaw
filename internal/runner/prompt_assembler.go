package runner

import (
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/internal/prompt"
	runnermodel "github.com/similarityyoung/simiclaw/internal/runner/model"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type llmPromptAssembly struct {
	chatMessages []kernel.ModelMessage
	toolDefs     []kernel.ToolDefinition
}

type llmPromptAssembler struct {
	prompts *prompt.Builder
	tools   kernel.ToolCatalog
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

	chatMessages := make([]kernel.ModelMessage, 0, len(history)+2)
	chatMessages = append(chatMessages, kernel.ModelMessage{Role: "system", Content: systemPrompt})
	chatMessages = append(chatMessages, historyToChatMessages(history)...)
	chatMessages = append(chatMessages, kernel.ModelMessage{Role: "user", Content: userText})

	toolDefs := make([]kernel.ToolDefinition, 0)
	for _, def := range a.tools.ToolDefinitions() {
		if !toolAllowed(def.Name, allowedTools) {
			continue
		}
		toolDefs = append(toolDefs, def)
	}

	return llmPromptAssembly{
		chatMessages: chatMessages,
		toolDefs:     toolDefs,
	}
}
