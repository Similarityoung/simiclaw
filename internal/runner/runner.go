package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/similarityyoung/simiclaw/internal/memory"
	"github.com/similarityyoung/simiclaw/internal/prompt"
	"github.com/similarityyoung/simiclaw/internal/provider"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/tools"
)

type Runner interface {
	Run(ctx context.Context, event model.InternalEvent, maxToolRounds int, sink StreamSink) (RunOutput, error)
}

type StreamSink interface {
	OnStatus(status, message string)
	OnReasoningDelta(delta string)
	OnTextDelta(delta string)
	OnToolStart(toolCallID, toolName string, args map[string]any, truncated bool)
	OnToolResult(toolCallID, toolName string, result map[string]any, truncated bool, apiErr *model.ErrorBlock)
}

type OutputMessage struct {
	Role       string
	Content    string
	Visible    bool
	ToolCallID string
	ToolName   string
	ToolArgs   map[string]any
	ToolResult map[string]any
	Meta       map[string]any
}

type RunOutput struct {
	RunMode        model.RunMode
	Messages       []OutputMessage
	Trace          model.RunTrace
	AssistantReply string
	SuppressOutput bool
}

type ProviderRunner struct {
	workspace    string
	db           *store.DB
	registry     *tools.Registry
	providers    *provider.Factory
	writer       *memory.Writer
	prompts      *prompt.Builder
	historyLimit int
}

var idSeq atomic.Uint64

func NewProviderRunner(workspace string, db *store.DB, registry *tools.Registry, providers *provider.Factory) *ProviderRunner {
	if registry == nil {
		registry = tools.NewRegistry()
		tools.RegisterBuiltins(registry)
	}
	return &ProviderRunner{
		workspace:    workspace,
		db:           db,
		registry:     registry,
		providers:    providers,
		writer:       memory.NewWriter(workspace),
		prompts:      prompt.NewBuilder(workspace),
		historyLimit: 20,
	}
}

func (r *ProviderRunner) Run(ctx context.Context, event model.InternalEvent, maxToolRounds int, sink StreamSink) (RunOutput, error) {
	start := time.Now().UTC()
	runMode := model.RunModeNormal
	if isNoReplyPayload(event.Payload.Type) {
		runMode = model.RunModeNoReply
	}
	trace := model.RunTrace{
		EventID:    event.EventID,
		SessionKey: event.SessionKey,
		SessionID:  event.ActiveSessionID,
		RunMode:    runMode,
		Status:     model.RunStatusCompleted,
		StartedAt:  start,
	}
	safeSink := newSafeStreamSink(sink, &trace)
	if runMode == model.RunModeNoReply {
		return r.runNoReply(event, start, trace)
	}
	return r.runInteractive(ctx, event, maxToolRounds, start, &trace, safeSink)
}

func (r *ProviderRunner) runNoReply(event model.InternalEvent, now time.Time, trace model.RunTrace) (RunOutput, error) {
	note := strings.TrimSpace(event.Payload.Text)
	if note == "" {
		note = event.Payload.Type
	}
	switch event.Payload.Type {
	case "memory_flush", "cron_fire":
		_, _ = r.writer.WriteDaily("system:"+event.Payload.Type, note, now)
	case "compaction":
		_, _ = r.writer.WriteCurated(note, now)
	}
	trace.OutputText = note
	trace.FinishedAt = time.Now().UTC()
	trace.LatencyMS = time.Since(now).Milliseconds()
	return RunOutput{
		RunMode: runModeForPayload(event.Payload.Type),
		Messages: []OutputMessage{{
			Role:    "system",
			Content: note,
			Visible: false,
			Meta:    map[string]any{"payload_type": event.Payload.Type},
		}},
		Trace:          trace,
		AssistantReply: "",
		SuppressOutput: true,
	}, nil
}

func (r *ProviderRunner) runInteractive(ctx context.Context, event model.InternalEvent, maxToolRounds int, now time.Time, trace *model.RunTrace, sink StreamSink) (RunOutput, error) {
	defaultModel := r.providers.DefaultModel()
	llmProvider, actualModel, err := r.providers.Resolve(defaultModel)
	if err != nil {
		trace.Status = model.RunStatusFailed
		trace.Error = &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: err.Error()}
		trace.FinishedAt = time.Now().UTC()
		trace.LatencyMS = time.Since(now).Milliseconds()
		return RunOutput{RunMode: model.RunModeNormal, Trace: *trace}, err
	}

	history, err := r.db.RecentMessages(ctx, event.ActiveSessionID, r.historyLimit)
	if err != nil {
		trace.Status = model.RunStatusFailed
		trace.Error = &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: err.Error()}
		trace.FinishedAt = time.Now().UTC()
		trace.LatencyMS = time.Since(now).Milliseconds()
		return RunOutput{RunMode: model.RunModeNormal, Trace: *trace}, err
	}
	ragHits, _ := r.db.SearchMessagesFTS(ctx, event.ActiveSessionID, strings.TrimSpace(event.Payload.Text), 5)
	trace.ContextManifest = &model.ContextManifest{
		HistoryRange: model.HistoryRange{Mode: "tail", TailLimit: r.historyLimit},
	}
	trace.RAGHits = ragHits

	messages := []OutputMessage{{
		Role:    "user",
		Content: strings.TrimSpace(event.Payload.Text),
		Visible: true,
	}}
	systemPrompt := r.prompts.Build(prompt.BuildInput{Context: prompt.RunContext{
		Now:          now,
		Conversation: event.Conversation,
		SessionKey:   event.SessionKey,
		SessionID:    event.ActiveSessionID,
		PayloadType:  event.Payload.Type,
	}})
	chatMessages := make([]provider.ChatMessage, 0, len(history)+2)
	chatMessages = append(chatMessages, provider.ChatMessage{Role: "system", Content: systemPrompt})
	for _, msg := range history {
		chatMessages = append(chatMessages, provider.ChatMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		})
	}
	chatMessages = append(chatMessages, provider.ChatMessage{Role: "user", Content: strings.TrimSpace(event.Payload.Text)})

	toolDefs := make([]provider.ToolDefinition, 0)
	for _, def := range r.registry.Definitions() {
		toolDefs = append(toolDefs, provider.ToolDefinition{
			Name:        def.Schema.Name,
			Description: def.Schema.Description,
			Parameters:  def.Schema.Parameters,
		})
	}

	var (
		totalUsage provider.Usage
		reply      string
		last       provider.ChatResult
	)
	for round := 0; round <= maxToolRounds; round++ {
		last, err = llmProvider.StreamChat(ctx, provider.ChatRequest{
			Model:    actualModel,
			Messages: chatMessages,
			Tools:    toolDefs,
		}, providerStreamSink{sink: sink})
		if err != nil {
			trace.Status = model.RunStatusFailed
			trace.Error = &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: err.Error()}
			trace.FinishedAt = time.Now().UTC()
			trace.LatencyMS = time.Since(now).Milliseconds()
			return RunOutput{
				RunMode:  model.RunModeNormal,
				Messages: messages,
				Trace:    *trace,
			}, err
		}
		totalUsage.PromptTokens += last.Usage.PromptTokens
		totalUsage.CompletionTokens += last.Usage.CompletionTokens
		totalUsage.TotalTokens += last.Usage.TotalTokens
		if len(last.ToolCalls) == 0 {
			reply = strings.TrimSpace(last.Text)
			break
		}
		if round == maxToolRounds {
			reply = strings.TrimSpace(last.Text)
			if reply == "" {
				reply = "工具调用轮次已达上限。"
			}
			break
		}

		chatMessages = append(chatMessages, provider.ChatMessage{
			Role:      "assistant",
			ToolCalls: last.ToolCalls,
		})
		for _, call := range last.ToolCalls {
			displayArgs, argsTruncated := sanitizeDisplayMap(call.Args)
			sink.OnToolStart(call.ToolCallID, call.Name, displayArgs, argsTruncated)
			res := callToolSafely(ctx, r.registry, tools.Context{
				Workspace:    r.workspace,
				Conversation: event.Conversation,
			}, call.Name, call.Args)
			exec := model.ToolExecution{
				ToolCallID: call.ToolCallID,
				Name:       call.Name,
				Args:       call.Args,
				Result:     res.Output,
				Error:      res.Error,
			}
			trace.ToolExecutions = append(trace.ToolExecutions, exec)
			trace.Actions = append(trace.Actions, model.Action{
				ActionID:             nextID("act", now),
				ActionIndex:          len(trace.Actions),
				ActionIdempotencyKey: fmt.Sprintf("%s:%d", event.EventID, len(trace.Actions)),
				Type:                 "InvokeTool",
				Risk:                 "low",
				Payload:              map[string]any{"tool_name": call.Name},
			})
			payload := map[string]any{}
			if res.Output != nil {
				payload = res.Output
			}
			displayResult, resultTruncated := sanitizeDisplayMap(payload)
			sink.OnToolResult(call.ToolCallID, call.Name, displayResult, resultTruncated, res.Error)
			content := toolResultString(res.Output, res.Error)
			messages = append(messages, OutputMessage{
				Role:       "tool",
				Content:    content,
				Visible:    true,
				ToolCallID: call.ToolCallID,
				ToolName:   call.Name,
				ToolArgs:   call.Args,
				ToolResult: payload,
			})
			chatMessages = append(chatMessages, provider.ChatMessage{
				Role:       "tool",
				Content:    content,
				ToolCallID: call.ToolCallID,
			})
		}
	}

	if reply == "" {
		reply = strings.TrimSpace(last.Text)
	}
	if reply != "" {
		messages = append(messages, OutputMessage{
			Role:    "assistant",
			Content: reply,
			Visible: true,
		})
	}

	trace.Provider = last.Provider
	trace.Model = last.Model
	trace.PromptTokens = totalUsage.PromptTokens
	trace.CompletionTokens = totalUsage.CompletionTokens
	trace.TotalTokens = totalUsage.TotalTokens
	trace.FinishReason = last.FinishReason
	trace.RawFinishReason = last.RawFinishReason
	trace.ProviderRequestID = last.ProviderRequestID
	trace.OutputText = reply
	trace.ToolCalls = last.ToolCalls
	trace.FinishedAt = time.Now().UTC()
	trace.LatencyMS = time.Since(now).Milliseconds()

	return RunOutput{
		RunMode:        model.RunModeNormal,
		Messages:       messages,
		Trace:          *trace,
		AssistantReply: reply,
		SuppressOutput: false,
	}, nil
}

func nextID(prefix string, now time.Time) string {
	n := idSeq.Add(1)
	return fmt.Sprintf("%s_%d_%04d", prefix, now.UnixNano(), n)
}

type providerStreamSink struct {
	sink StreamSink
}

func (s providerStreamSink) OnReasoningDelta(delta string) {
	if s.sink != nil {
		s.sink.OnReasoningDelta(delta)
	}
}

func (s providerStreamSink) OnTextDelta(delta string) {
	if s.sink != nil {
		s.sink.OnTextDelta(delta)
	}
}

type safeStreamSink struct {
	inner StreamSink
	trace *model.RunTrace
}

func newSafeStreamSink(inner StreamSink, trace *model.RunTrace) StreamSink {
	if inner == nil {
		return noopStreamSink{}
	}
	return safeStreamSink{inner: inner, trace: trace}
}

func (s safeStreamSink) OnStatus(status, message string) {
	s.call("status", func() { s.inner.OnStatus(status, message) })
}

func (s safeStreamSink) OnReasoningDelta(delta string) {
	s.call("reasoning_delta", func() { s.inner.OnReasoningDelta(delta) })
}

func (s safeStreamSink) OnTextDelta(delta string) {
	s.call("text_delta", func() { s.inner.OnTextDelta(delta) })
}

func (s safeStreamSink) OnToolStart(toolCallID, toolName string, args map[string]any, truncated bool) {
	s.call("tool_start", func() { s.inner.OnToolStart(toolCallID, toolName, args, truncated) })
}

func (s safeStreamSink) OnToolResult(toolCallID, toolName string, result map[string]any, truncated bool, apiErr *model.ErrorBlock) {
	s.call("tool_result", func() { s.inner.OnToolResult(toolCallID, toolName, result, truncated, apiErr) })
}

func (s safeStreamSink) call(kind string, fn func()) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if s.trace.Diagnostics == nil {
				s.trace.Diagnostics = map[string]string{}
			}
			s.trace.Diagnostics["stream_sink."+kind] = fmt.Sprintf("panic: %v", recovered)
		}
	}()
	fn()
}

type noopStreamSink struct{}

func (noopStreamSink) OnStatus(string, string) {}

func (noopStreamSink) OnReasoningDelta(string) {}

func (noopStreamSink) OnTextDelta(string) {}

func (noopStreamSink) OnToolStart(string, string, map[string]any, bool) {}

func (noopStreamSink) OnToolResult(string, string, map[string]any, bool, *model.ErrorBlock) {}

func callToolSafely(ctx context.Context, registry *tools.Registry, toolCtx tools.Context, name string, args map[string]any) (result tools.Result) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = tools.Result{
				Error: &model.ErrorBlock{
					Code:    model.ErrorCodeInternal,
					Message: fmt.Sprintf("tool panic: %v", recovered),
				},
			}
		}
	}()
	return registry.Call(ctx, toolCtx, name, args)
}

const (
	maxDisplayDepth    = 4
	maxDisplayItems    = 20
	maxDisplayRuneSize = 256
)

func sanitizeDisplayMap(input map[string]any) (map[string]any, bool) {
	if len(input) == 0 {
		return map[string]any{}, false
	}
	truncated := false
	out := sanitizeDisplayValue(input, 0, &truncated, "").(map[string]any)
	return out, truncated
}

func sanitizeDisplayValue(input any, depth int, truncated *bool, key string) any {
	if depth >= maxDisplayDepth {
		*truncated = true
		return "[truncated]"
	}
	switch value := input.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		count := 0
		for childKey, childValue := range value {
			if count >= maxDisplayItems {
				out["_truncated_items"] = true
				*truncated = true
				break
			}
			if isSensitiveKey(childKey) {
				out[childKey] = "[redacted]"
				count++
				continue
			}
			out[childKey] = sanitizeDisplayValue(childValue, depth+1, truncated, childKey)
			count++
		}
		return out
	case []any:
		limit := len(value)
		if limit > maxDisplayItems {
			limit = maxDisplayItems
			*truncated = true
		}
		out := make([]any, 0, limit)
		for i := 0; i < limit; i++ {
			out = append(out, sanitizeDisplayValue(value[i], depth+1, truncated, key))
		}
		return out
	case string:
		if isSensitiveKey(key) {
			return "[redacted]"
		}
		runes := []rune(value)
		if len(runes) <= maxDisplayRuneSize {
			return value
		}
		*truncated = true
		return string(runes[:maxDisplayRuneSize]) + "..."
	default:
		return input
	}
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "token") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "password") ||
		strings.Contains(key, "api_key") ||
		strings.Contains(key, "apikey") ||
		strings.Contains(key, "authorization")
}

func toolResultString(output map[string]any, apiErr *model.ErrorBlock) string {
	if apiErr != nil {
		return apiErr.Code + ": " + apiErr.Message
	}
	if len(output) == 0 {
		return "{}"
	}
	b, err := json.Marshal(output)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func runModeForPayload(payloadType string) model.RunMode {
	if isNoReplyPayload(payloadType) {
		return model.RunModeNoReply
	}
	return model.RunModeNormal
}

func isNoReplyPayload(payloadType string) bool {
	return payloadType == "memory_flush" || payloadType == "compaction" || payloadType == "cron_fire"
}
