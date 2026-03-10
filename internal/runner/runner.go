package runner

import (
	"context"
	"strings"
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
	registry        *tools.Registry
	providers       *provider.Factory
	writer          *memory.Writer
	historyLoader   runHistoryLoader
	promptAssembler llmPromptAssembler
	toolExecutor    llmToolExecutor
	traceAssembler  runTraceAssembler
}

func NewProviderRunner(workspace string, db *store.DB, registry *tools.Registry, providers *provider.Factory) *ProviderRunner {
	if registry == nil {
		registry = tools.NewRegistry()
		tools.RegisterBuiltins(registry)
	}
	prompts := prompt.NewBuilder(workspace)
	runner := &ProviderRunner{
		registry:  registry,
		providers: providers,
		writer:    memory.NewWriter(workspace),
	}
	runner.historyLoader = runHistoryLoader{db: db, historyLimit: 20}
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
