package runner

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/internal/prompt"
	"github.com/similarityyoung/simiclaw/internal/provider"
	runnercontext "github.com/similarityyoung/simiclaw/internal/runner/context"
	runnermodel "github.com/similarityyoung/simiclaw/internal/runner/model"
	runtimepayload "github.com/similarityyoung/simiclaw/internal/runtime/payload"
	"github.com/similarityyoung/simiclaw/internal/tools"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
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
	Trace          api.RunTrace
	AssistantReply string
	SuppressOutput bool
}

type HistoryReader interface {
	LoadPromptHistory(ctx context.Context, sessionID string, limit int) ([]runnermodel.HistoryMessage, error)
	SearchRAGHits(ctx context.Context, sessionID, query string, limit int) ([]runnermodel.RAGHit, error)
}

type ProviderRunner struct {
	registry        *tools.Registry
	providers       *provider.Factory
	memoryWriter    runnercontext.MemoryWriter
	contextBuilder  runnercontext.Assembler
	promptAssembler llmPromptAssembler
	toolExecutor    llmToolExecutor
	traceAssembler  runTraceAssembler
	payloads        *runtimepayload.Registry
}

func NewProviderRunner(workspace string, historyReader HistoryReader, registry *tools.Registry, providers *provider.Factory, payloads *runtimepayload.Registry) *ProviderRunner {
	if registry == nil {
		registry = tools.NewRegistry()
		tools.RegisterBuiltins(registry)
	}
	prompts := prompt.NewBuilder(workspace)
	runner := &ProviderRunner{
		registry:     registry,
		providers:    providers,
		memoryWriter: runnercontext.NewMemoryWriter(workspace),
		payloads:     payloads,
	}
	runner.contextBuilder = runnercontext.NewAssembler(historyReader)
	runner.promptAssembler = llmPromptAssembler{prompts: prompts, registry: runner.registry}
	runner.toolExecutor = llmToolExecutor{workspace: workspace, registry: runner.registry}
	runner.traceAssembler = runTraceAssembler{}
	return runner
}

func (r *ProviderRunner) Run(ctx context.Context, event model.InternalEvent, maxToolRounds int, sink StreamSink) (RunOutput, error) {
	start := time.Now().UTC()
	if event.Payload.Type == payloadTypeNewSession {
		trace := api.RunTrace{
			EventID:    event.EventID,
			SessionKey: event.SessionKey,
			SessionID:  event.ActiveSessionID,
			RunMode:    model.RunModeNormal,
			Status:     model.RunStatusCompleted,
			StartedAt:  start,
		}
		return runNewSession(event, start, &trace), nil
	}
	if r.payloads == nil {
		return RunOutput{}, errors.New("payload registry unavailable")
	}
	plan := r.payloads.Resolve(event.Payload.Type)
	trace := api.RunTrace{
		EventID:    event.EventID,
		SessionKey: event.SessionKey,
		SessionID:  event.ActiveSessionID,
		RunMode:    plan.RunMode,
		Status:     model.RunStatusCompleted,
		StartedAt:  start,
	}
	return r.runWithPlan(ctx, event, maxToolRounds, start, &trace, sink, plan)
}

func (r *ProviderRunner) runWithPlan(ctx context.Context, event model.InternalEvent, maxToolRounds int, now time.Time, trace *api.RunTrace, sink StreamSink, plan runtimepayload.Plan) (RunOutput, error) {
	switch plan.Kind {
	case runtimepayload.ExecutionKindMemoryWrite:
		return r.runMemoryWrite(event, now, trace, plan)
	default:
		streamSink := sink
		if plan.SuppressStream {
			streamSink = nil
		}
		return r.runLLM(ctx, event, maxToolRounds, now, trace, newSafeStreamSink(streamSink, trace), llmRunOptions{
			runMode:               plan.RunMode,
			suppressOutput:        plan.SuppressOutput,
			userVisible:           plan.UserVisible,
			toolVisible:           plan.ToolVisible,
			finalAssistantVisible: plan.FinalAssistantVisible,
			messageMeta:           cloneMap(plan.MessageMeta),
			allowedTools:          plan.AllowedTools,
		})
	}
}

func (r *ProviderRunner) runMemoryWrite(event model.InternalEvent, now time.Time, trace *api.RunTrace, plan runtimepayload.Plan) (RunOutput, error) {
	note := strings.TrimSpace(event.Payload.Text)
	if note == "" {
		note = event.Payload.Type
	}
	switch plan.MemoryWriteTarget {
	case runtimepayload.MemoryWriteTargetDaily:
		_ = r.memoryWriter.WriteDaily("system:"+event.Payload.Type, note, now, event.Conversation.ChannelType)
	case runtimepayload.MemoryWriteTargetCurated:
		_ = r.memoryWriter.WriteCurated(note, now, event.Conversation.ChannelType)
	}
	trace.OutputText = note
	trace.FinishedAt = time.Now().UTC()
	trace.LatencyMS = time.Since(now).Milliseconds()
	return RunOutput{
		RunMode: plan.RunMode,
		Messages: []OutputMessage{{
			Role:    "system",
			Content: note,
			Visible: false,
			Meta:    cloneMap(plan.MessageMeta),
		}},
		Trace:          *trace,
		AssistantReply: "",
		SuppressOutput: plan.SuppressOutput,
	}, nil
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

func (r *ProviderRunner) runLLM(ctx context.Context, event model.InternalEvent, maxToolRounds int, now time.Time, trace *api.RunTrace, sink StreamSink, opts llmRunOptions) (RunOutput, error) {
	defaultModel := r.providers.DefaultModel()
	llmProvider, actualModel, err := r.providers.Resolve(defaultModel)
	if err != nil {
		r.traceAssembler.Fail(trace, now, err)
		return RunOutput{RunMode: opts.runMode, Trace: *trace}, err
	}

	history, err := r.contextBuilder.Load(ctx, event.ActiveSessionID, event.Payload.Text)
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
	assembly := r.promptAssembler.Assemble(event, now, history.History, opts.allowedTools)
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

func runNewSession(event model.InternalEvent, now time.Time, trace *api.RunTrace) RunOutput {
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
