package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type openAICompatibleProvider struct {
	name           string
	client         openai.Client
	requestTimeout time.Duration
}

func newOpenAICompatibleProvider(name string, cfg config.LLMProviderConfig) (LLMProvider, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("openai-compatible provider requires api_key")
	}
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithHTTPClient(&http.Client{}),
	}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &openAICompatibleProvider{
		name:           name,
		client:         openai.NewClient(opts...),
		requestTimeout: cfg.Timeout.Duration,
	}, nil
}

func (p *openAICompatibleProvider) Chat(ctx context.Context, req ChatRequest) (ChatResult, error) {
	params := buildChatCompletionParams(req, false)

	resp, err := p.client.Chat.Completions.New(ctx, params, p.requestTimeoutOption()...)
	if err != nil {
		err = kernel.WrapCapabilityError("provider", p.name, "chat", err)
		logTransportDebug(p.name, req.Model, "chat transport failed", p.requestTimeout, err)
		return ChatResult{}, err
	}
	if len(resp.Choices) == 0 {
		return ChatResult{}, kernel.NewCapabilityError("provider", p.name, "chat", kernel.CapabilityErrorInvalidResponse, errors.New("openai-compatible provider returned no choices"))
	}
	return chatResultFromCompletion(p.name, resp)
}

func (p *openAICompatibleProvider) StreamChat(ctx context.Context, req ChatRequest, sink StreamSink) (ChatResult, error) {
	params := buildChatCompletionParams(req, true)
	stream := p.client.Chat.Completions.NewStreaming(ctx, params, p.requestTimeoutOption()...)
	acc := openai.ChatCompletionAccumulator{}
	toolAcc := newToolCallAccumulator()
	for stream.Next() {
		chunk := stream.Current()
		if !acc.AddChunk(chunk) {
			err := kernel.NewCapabilityError("provider", p.name, "stream_chat", kernel.CapabilityErrorInvalidResponse, errors.New("openai-compatible provider returned inconsistent streaming chunks"))
			logTransportDebug(p.name, req.Model, "stream chunk rejected", p.requestTimeout, err)
			return ChatResult{}, err
		}
		toolAcc.AddChunk(chunk)
		if sink != nil {
			for _, choice := range chunk.Choices {
				if choice.Index != 0 {
					continue
				}
				if delta := choice.Delta.Content; delta != "" {
					sink.OnTextDelta(delta)
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		err = kernel.WrapCapabilityError("provider", p.name, "stream_chat", err)
		logTransportDebug(p.name, req.Model, "stream transport failed", p.requestTimeout, err)
		return ChatResult{}, err
	}
	result, err := chatResultFromAccumulator(p.name, &acc, toolAcc)
	if err != nil {
		logTransportDebug(p.name, req.Model, "stream accumulator failed", p.requestTimeout, err)
		return ChatResult{}, err
	}
	return result, nil
}

func (p *openAICompatibleProvider) requestTimeoutOption() []option.RequestOption {
	if p.requestTimeout <= 0 {
		return nil
	}
	return []option.RequestOption{option.WithRequestTimeout(p.requestTimeout)}
}

func buildChatCompletionParams(req ChatRequest, includeUsage bool) openai.ChatCompletionNewParams {
	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(req.Model),
		Messages: buildChatMessages(req.Messages),
	}
	if len(req.Tools) > 0 {
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.String("auto"),
		}
		params.Tools = buildToolParams(req.Tools)
	}
	if includeUsage {
		params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		}
	}
	return params
}

func chatResultFromCompletion(providerName string, resp *openai.ChatCompletion) (ChatResult, error) {
	choice := resp.Choices[0]
	result := ChatResult{
		Text:              choice.Message.Content,
		FinishReason:      choice.FinishReason,
		RawFinishReason:   choice.FinishReason,
		Provider:          providerName,
		Model:             resp.Model,
		ProviderRequestID: resp.ID,
		Usage: Usage{
			PromptTokens:     int(resp.Usage.PromptTokens),
			CompletionTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:      int(resp.Usage.TotalTokens),
		},
	}
	for _, toolCall := range choice.Message.ToolCalls {
		fn := toolCall.AsFunction()
		args := map[string]any{}
		if fn.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(fn.Function.Arguments), &args)
		}
		result.ToolCalls = append(result.ToolCalls, model.ToolCall{
			ToolCallID: fn.ID,
			Name:       fn.Function.Name,
			Args:       args,
		})
	}
	return result, nil
}

func buildChatMessages(messages []ChatMessage) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "assistant":
			item := openai.ChatCompletionAssistantMessageParam{}
			if strings.TrimSpace(msg.Content) != "" {
				item.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.String(msg.Content),
				}
			}
			if len(msg.ToolCalls) > 0 {
				item.ToolCalls = make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(msg.ToolCalls))
				for _, call := range msg.ToolCalls {
					item.ToolCalls = append(item.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: call.ToolCallID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      call.Name,
								Arguments: mustJSONString(call.Args),
							},
						},
					})
				}
			}
			out = append(out, openai.ChatCompletionMessageParamUnion{OfAssistant: &item})
		case "tool":
			item := openai.ChatCompletionToolMessageParam{
				Content: openai.ChatCompletionToolMessageParamContentUnion{
					OfString: openai.String(msg.Content),
				},
				ToolCallID: msg.ToolCallID,
			}
			out = append(out, openai.ChatCompletionMessageParamUnion{OfTool: &item})
		case "system":
			item := openai.ChatCompletionSystemMessageParam{
				Content: openai.ChatCompletionSystemMessageParamContentUnion{
					OfString: openai.String(msg.Content),
				},
			}
			out = append(out, openai.ChatCompletionMessageParamUnion{OfSystem: &item})
		default:
			item := openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfString: openai.String(msg.Content),
				},
			}
			out = append(out, openai.ChatCompletionMessageParamUnion{OfUser: &item})
		}
	}
	return out
}

func buildToolParams(defs []ToolDefinition) []openai.ChatCompletionToolUnionParam {
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(defs))
	for _, def := range defs {
		out = append(out, openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: shared.FunctionDefinitionParam{
					Name:        def.Name,
					Description: openai.String(def.Description),
					Parameters:  shared.FunctionParameters(schemaToMap(def.Parameters)),
					Strict:      openai.Bool(true),
				},
			},
		})
	}
	return out
}

func schemaToMap(schema any) map[string]any {
	b, err := json.Marshal(schema)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func mustJSONString(v any) string {
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
