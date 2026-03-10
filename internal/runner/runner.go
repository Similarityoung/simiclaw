package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
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
	ToolCalls  []model.ToolCall
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
	workspace       string
	db              *store.DB
	registry        *tools.Registry
	providers       *provider.Factory
	writer          *memory.Writer
	prompts         *prompt.Builder
	historyLimit    int
	historyLoader   runHistoryLoader
	promptAssembler llmPromptAssembler
	toolExecutor    llmToolExecutor
	traceAssembler  runTraceAssembler
}

var idSeq atomic.Uint64

func NewProviderRunner(workspace string, db *store.DB, registry *tools.Registry, providers *provider.Factory) *ProviderRunner {
	if registry == nil {
		registry = tools.NewRegistry()
		tools.RegisterBuiltins(registry)
	}
	prompts := prompt.NewBuilder(workspace)
	runner := &ProviderRunner{
		workspace:    workspace,
		db:           db,
		registry:     registry,
		providers:    providers,
		writer:       memory.NewWriter(workspace),
		prompts:      prompts,
		historyLimit: 20,
	}
	runner.historyLoader = runHistoryLoader{db: db, historyLimit: runner.historyLimit}
	runner.promptAssembler = llmPromptAssembler{prompts: prompts, registry: runner.registry}
	runner.toolExecutor = llmToolExecutor{workspace: workspace, registry: runner.registry}
	runner.traceAssembler = runTraceAssembler{}
	return runner
}

func (r *ProviderRunner) Run(ctx context.Context, event model.InternalEvent, maxToolRounds int, sink StreamSink) (RunOutput, error) {
	start := time.Now().UTC()
	if event.Payload.Type == payloadTypeNewSession {
		trace := model.RunTrace{
			EventID:    event.EventID,
			SessionKey: event.SessionKey,
			SessionID:  event.ActiveSessionID,
			RunMode:    model.RunModeNormal,
			Status:     model.RunStatusCompleted,
			StartedAt:  start,
		}
		return runNewSession(event, start, &trace), nil
	}
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
		return r.runNoReply(ctx, event, maxToolRounds, start, &trace, safeSink)
	}
	return r.runInteractive(ctx, event, maxToolRounds, start, &trace, safeSink)
}

func (r *ProviderRunner) runNoReply(ctx context.Context, event model.InternalEvent, maxToolRounds int, now time.Time, trace *model.RunTrace, sink StreamSink) (RunOutput, error) {
	if event.Payload.Type == "cron_fire" {
		return r.runSuppressedCronFire(ctx, event, maxToolRounds, now, trace)
	}

	note := strings.TrimSpace(event.Payload.Text)
	if note == "" {
		note = event.Payload.Type
	}
	visibility := memory.VisibilityForChannel(event.Conversation.ChannelType)
	switch event.Payload.Type {
	case "memory_flush":
		_, _ = r.writer.WriteDaily("system:"+event.Payload.Type, note, now, visibility)
	case "compaction":
		_, _ = r.writer.WriteCurated(note, now, visibility)
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
			Meta:    map[string]any{payloadTypeMetaKey: event.Payload.Type},
		}},
		Trace:          *trace,
		AssistantReply: "",
		SuppressOutput: true,
	}, nil
}

func (r *ProviderRunner) runSuppressedCronFire(ctx context.Context, event model.InternalEvent, maxToolRounds int, now time.Time, trace *model.RunTrace) (RunOutput, error) {
	return r.runLLM(ctx, event, maxToolRounds, now, trace, newSafeStreamSink(nil, trace), llmRunOptions{
		runMode:               model.RunModeNoReply,
		suppressOutput:        true,
		userVisible:           false,
		toolVisible:           false,
		finalAssistantVisible: false,
		messageMeta:           map[string]any{payloadTypeMetaKey: event.Payload.Type},
		allowedTools:          cronFireAllowedTools,
	})
}

func (r *ProviderRunner) runInteractive(ctx context.Context, event model.InternalEvent, maxToolRounds int, now time.Time, trace *model.RunTrace, sink StreamSink) (RunOutput, error) {
	return r.runLLM(ctx, event, maxToolRounds, now, trace, sink, llmRunOptions{
		runMode:               model.RunModeNormal,
		suppressOutput:        false,
		userVisible:           true,
		toolVisible:           true,
		finalAssistantVisible: true,
	})
}

type llmRunOptions struct {
	runMode               model.RunMode
	suppressOutput        bool
	userVisible           bool
	toolVisible           bool
	finalAssistantVisible bool
	messageMeta           map[string]any
	allowedTools          map[string]struct{}
}

const payloadTypeMetaKey = "payload_type"
const payloadTypeNewSession = "new_session"

var cronFireAllowedTools = map[string]struct{}{
	"memory_search": {},
	"memory_get":    {},
	"context_get":   {},
}

var cronFireToolBudgets = map[string]int{
	"memory_search": 1,
	"memory_get":    1,
	"context_get":   1,
}

var cronFireInjectedRootFiles = map[string]struct{}{
	"SOUL.md":      {},
	"IDENTITY.md":  {},
	"USER.md":      {},
	"AGENTS.md":    {},
	"TOOLS.md":     {},
	"BOOTSTRAP.md": {},
	"HEARTBEAT.md": {},
}

func (r *ProviderRunner) runLLM(ctx context.Context, event model.InternalEvent, maxToolRounds int, now time.Time, trace *model.RunTrace, sink StreamSink, opts llmRunOptions) (RunOutput, error) {
	defaultModel := r.providers.DefaultModel()
	llmProvider, actualModel, err := r.providers.Resolve(defaultModel)
	if err != nil {
		r.traceAssembler.Fail(trace, now, err)
		return RunOutput{RunMode: opts.runMode, Trace: *trace}, err
	}

	history, err := r.historyLoader.Load(ctx, event.ActiveSessionID, event.Payload.Text)
	if err != nil {
		r.traceAssembler.Fail(trace, now, err)
		return RunOutput{RunMode: opts.runMode, Trace: *trace}, err
	}
	r.traceAssembler.AttachContext(trace, history)

	messages := []OutputMessage{{
		Role:    "user",
		Content: strings.TrimSpace(event.Payload.Text),
		Visible: opts.userVisible,
		Meta:    cloneMap(opts.messageMeta),
	}}
	assembly := r.promptAssembler.Assemble(event, now, history.history, opts.allowedTools)
	chatMessages := assembly.chatMessages
	toolDefs := assembly.toolDefs

	var (
		totalUsage    provider.Usage
		reply         string
		last          provider.ChatResult
		toolUseCounts = map[string]int{}
	)
	for round := 0; round <= maxToolRounds; round++ {
		last, err = llmProvider.StreamChat(ctx, provider.ChatRequest{
			Model:    actualModel,
			Messages: chatMessages,
			Tools:    toolDefs,
		}, providerStreamSink{sink: sink})
		if err != nil {
			r.traceAssembler.Fail(trace, now, err)
			return RunOutput{
				RunMode:  opts.runMode,
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
			reply = maxToolRoundsReply(last)
			break
		}

		assistantToolMessage := OutputMessage{
			Role:      "assistant",
			Content:   strings.TrimSpace(last.Text),
			Visible:   false,
			ToolCalls: cloneToolCalls(last.ToolCalls),
			Meta:      cloneMap(opts.messageMeta),
		}
		messages = append(messages, assistantToolMessage)
		chatMessages = append(chatMessages, provider.ChatMessage{
			Role:      "assistant",
			Content:   strings.TrimSpace(last.Text),
			ToolCalls: cloneToolCalls(last.ToolCalls),
		})
		for _, call := range last.ToolCalls {
			step := r.toolExecutor.Execute(ctx, event, call, opts, toolUseCounts, sink, len(trace.Actions), now)
			trace.ToolExecutions = append(trace.ToolExecutions, step.execution)
			trace.Actions = append(trace.Actions, step.action)
			messages = append(messages, step.message)
			chatMessages = append(chatMessages, provider.ChatMessage{
				Role:       step.chat.role,
				Content:    step.chat.content,
				ToolCallID: step.chat.toolCallID,
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
			Visible: opts.finalAssistantVisible,
			Meta:    cloneMap(opts.messageMeta),
		})
	}

	r.traceAssembler.Complete(trace, now, totalUsage, last, reply)

	assistantReply := reply
	if opts.suppressOutput {
		assistantReply = ""
	}

	return RunOutput{
		RunMode:        opts.runMode,
		Messages:       messages,
		Trace:          *trace,
		AssistantReply: assistantReply,
		SuppressOutput: opts.suppressOutput,
	}, nil
}

func maxToolRoundsReply(last provider.ChatResult) string {
	if len(last.ToolCalls) > 0 {
		return "工具调用轮次已达上限。"
	}
	reply := strings.TrimSpace(last.Text)
	if reply == "" {
		return "工具调用轮次已达上限。"
	}
	return reply
}

func cronFireToolPolicyError(call model.ToolCall, counts map[string]int) *model.ErrorBlock {
	if budget, ok := cronFireToolBudgets[call.Name]; ok && counts[call.Name] >= budget {
		return &model.ErrorBlock{
			Code:    model.ErrorCodeForbidden,
			Message: fmt.Sprintf("cron_fire tool budget exhausted for %q; summarize with current evidence instead of fetching more context", call.Name),
		}
	}
	if call.Name != "context_get" {
		return nil
	}
	path, _ := call.Args["path"].(string)
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(path))))
	if _, ok := cronFireInjectedRootFiles[clean]; ok {
		return &model.ErrorBlock{
			Code:    model.ErrorCodeForbidden,
			Message: fmt.Sprintf("context_get %q is already injected into the cron_fire system prompt; summarize with current evidence instead of rereading it", clean),
		}
	}
	return nil
}

func toolAllowed(name string, allowed map[string]struct{}) bool {
	if allowed == nil {
		return true
	}
	_, ok := allowed[name]
	return ok
}

func toolRisk(name string) string {
	switch name {
	case "workspace_patch":
		return "medium"
	case "workspace_delete":
		return "high"
	default:
		return "low"
	}
}

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

func runNewSession(event model.InternalEvent, now time.Time, trace *model.RunTrace) RunOutput {
	const reply = "已开始新会话。"
	meta := map[string]any{payloadTypeMetaKey: event.Payload.Type}
	trace.OutputText = reply
	trace.FinishedAt = time.Now().UTC()
	trace.LatencyMS = time.Since(now).Milliseconds()
	return RunOutput{
		RunMode: model.RunModeNormal,
		Messages: []OutputMessage{
			{Role: "user", Content: strings.TrimSpace(event.Payload.Text), Visible: true, Meta: cloneMap(meta)},
			{Role: "assistant", Content: reply, Visible: true, Meta: cloneMap(meta)},
		},
		Trace:          *trace,
		AssistantReply: reply,
	}
}

func shouldSkipPromptHistoryPayloadType(payloadType any) bool {
	value, _ := payloadType.(string)
	return value == "cron_fire" || value == payloadTypeNewSession
}

func cloneToolCalls(in []model.ToolCall) []model.ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.ToolCall, 0, len(in))
	for _, call := range in {
		cloned := call
		if call.Args != nil {
			cloned.Args = cloneMap(call.Args)
		}
		out = append(out, cloned)
	}
	return out
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
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
