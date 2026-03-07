package provider

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	openai "github.com/openai/openai-go/v3"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type toolAccumulatorKey struct {
	ChoiceIndex int
	ToolIndex   int
}

type accumulatedToolCall struct {
	choiceIndex int
	toolIndex   int
	toolCallID  string
	name        strings.Builder
	arguments   strings.Builder
}

type toolCallAccumulator struct {
	byKey map[toolAccumulatorKey]*accumulatedToolCall
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{
		byKey: make(map[toolAccumulatorKey]*accumulatedToolCall),
	}
}

func (a *toolCallAccumulator) AddChunk(chunk openai.ChatCompletionChunk) {
	for _, choice := range chunk.Choices {
		choiceIndex := int(choice.Index)
		for _, deltaTool := range choice.Delta.ToolCalls {
			key := toolAccumulatorKey{
				ChoiceIndex: choiceIndex,
				ToolIndex:   clampToolIndex(deltaTool.Index),
			}
			state, ok := a.byKey[key]
			if !ok {
				state = &accumulatedToolCall{
					choiceIndex: choiceIndex,
					toolIndex:   key.ToolIndex,
				}
				a.byKey[key] = state
			}
			if deltaTool.ID != "" {
				state.toolCallID = deltaTool.ID
			}
			if deltaTool.Function.Name != "" {
				_, _ = state.name.WriteString(deltaTool.Function.Name)
			}
			if deltaTool.Function.Arguments != "" {
				_, _ = state.arguments.WriteString(deltaTool.Function.Arguments)
			}
		}
	}
}

func (a *toolCallAccumulator) BuildToolCalls(choiceIndex int) ([]model.ToolCall, error) {
	states := make([]*accumulatedToolCall, 0)
	for _, state := range a.byKey {
		if state.choiceIndex == choiceIndex {
			states = append(states, state)
		}
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].toolIndex < states[j].toolIndex
	})
	out := make([]model.ToolCall, 0, len(states))
	for _, state := range states {
		rawArgs := strings.TrimSpace(state.arguments.String())
		args := map[string]any{}
		if rawArgs != "" {
			if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
				return nil, fmt.Errorf("invalid tool arguments for %q: %w", state.name.String(), err)
			}
		}
		toolCallID := state.toolCallID
		if toolCallID == "" {
			toolCallID = fmt.Sprintf("tool_call_%d", state.toolIndex)
		}
		out = append(out, model.ToolCall{
			ToolCallID: toolCallID,
			Name:       state.name.String(),
			Args:       args,
		})
	}
	return out, nil
}

func chatResultFromAccumulator(providerName string, acc *openai.ChatCompletionAccumulator, toolAcc *toolCallAccumulator) (ChatResult, error) {
	if acc == nil || len(acc.Choices) == 0 {
		return ChatResult{}, fmt.Errorf("openai-compatible provider returned no streaming choices")
	}
	choice := acc.Choices[0]
	toolCalls, err := toolAcc.BuildToolCalls(0)
	if err != nil {
		return ChatResult{}, err
	}
	return ChatResult{
		Text:              choice.Message.Content,
		ToolCalls:         toolCalls,
		FinishReason:      choice.FinishReason,
		RawFinishReason:   choice.FinishReason,
		Provider:          providerName,
		Model:             acc.Model,
		ProviderRequestID: acc.ID,
		Usage: Usage{
			PromptTokens:     int(acc.Usage.PromptTokens),
			CompletionTokens: int(acc.Usage.CompletionTokens),
			TotalTokens:      int(acc.Usage.TotalTokens),
		},
	}, nil
}

func clampToolIndex(index int64) int {
	if index < 0 {
		return 0
	}
	return int(index)
}
