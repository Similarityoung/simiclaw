package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type fakeProvider struct {
	name string
	cfg  config.LLMProviderConfig
}

func newProvider(name string, cfg config.LLMProviderConfig) (LLMProvider, error) {
	switch cfg.Type {
	case "fake":
		return &fakeProvider{name: name, cfg: cfg}, nil
	case "openai_compatible":
		return newOpenAICompatibleProvider(name, cfg)
	default:
		return nil, fmt.Errorf("unsupported provider type %q", cfg.Type)
	}
}

func (p *fakeProvider) Chat(_ context.Context, req ChatRequest) (ChatResult, error) {
	text := p.cfg.FakeResponseText
	if text == "" {
		text = "已收到: {{last_user_message}}"
	}
	lastUser := ""
	firstSystem := ""
	roles := make([]string, 0, len(req.Messages))
	seenToolResult := false
	for i := len(req.Messages) - 1; i >= 0; i-- {
		switch req.Messages[i].Role {
		case "user":
			if lastUser == "" {
				lastUser = req.Messages[i].Content
			}
		case "tool":
			seenToolResult = true
		}
	}
	for _, msg := range req.Messages {
		roles = append(roles, msg.Role)
		if msg.Role == "system" && firstSystem == "" {
			firstSystem = msg.Content
		}
	}
	text = strings.ReplaceAll(text, "{{last_user_message}}", lastUser)
	text = strings.ReplaceAll(text, "{{first_system_message}}", firstSystem)
	text = strings.ReplaceAll(text, "{{message_roles}}", strings.Join(roles, ","))

	var toolCalls []model.ToolCall
	if p.cfg.FakeToolName != "" && !seenToolResult {
		args := map[string]any{}
		if raw := strings.TrimSpace(p.cfg.FakeToolArgsJSON); raw != "" {
			_ = json.Unmarshal([]byte(raw), &args)
		}
		toolCalls = []model.ToolCall{{
			ToolCallID: "fake-tool-call-1",
			Name:       p.cfg.FakeToolName,
			Args:       args,
		}}
		text = ""
	}

	usage := Usage{
		PromptTokens:     p.cfg.FakePromptTokens,
		CompletionTokens: p.cfg.FakeCompletionTokens,
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return ChatResult{
		Text:              text,
		ToolCalls:         toolCalls,
		FinishReason:      p.cfg.FakeFinishReason,
		RawFinishReason:   p.cfg.FakeRawFinishReason,
		Usage:             usage,
		Provider:          p.name,
		Model:             req.Model,
		ProviderRequestID: p.cfg.FakeRequestID,
	}, nil
}

func (p *fakeProvider) StreamChat(ctx context.Context, req ChatRequest, sink StreamSink) (ChatResult, error) {
	result, err := p.Chat(ctx, req)
	if err != nil {
		return ChatResult{}, err
	}
	if sink == nil || strings.TrimSpace(result.Text) == "" {
		return result, ctx.Err()
	}
	for _, chunk := range splitRunes(result.Text, 4) {
		if err := ctx.Err(); err != nil {
			return ChatResult{}, err
		}
		sink.OnTextDelta(chunk)
	}
	return result, nil
}

func splitRunes(in string, chunkSize int) []string {
	if chunkSize <= 0 {
		chunkSize = 1
	}
	runes := []rune(in)
	out := make([]string, 0, (len(runes)+chunkSize-1)/chunkSize)
	for len(runes) > 0 {
		n := chunkSize
		if len(runes) < n {
			n = len(runes)
		}
		out = append(out, string(runes[:n]))
		runes = runes[n:]
	}
	return out
}
