package runner

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/internal/prompt"
	runnercontext "github.com/similarityyoung/simiclaw/internal/runner/context"
	runnermodel "github.com/similarityyoung/simiclaw/internal/runner/model"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	runtimepayload "github.com/similarityyoung/simiclaw/internal/runtime/payload"
	"github.com/similarityyoung/simiclaw/internal/tools"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/logging"
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
	tools          kernel.ToolCatalog
	payloads       *runtimepayload.Registry
	memoryExecutor memoryRunExecutor
	llmExecutor    llmAgentExecutor
}

func NewProviderRunner(workspace string, historyReader HistoryReader, toolCatalog kernel.ToolCatalog, providers kernel.ModelResolver, payloads *runtimepayload.Registry) *ProviderRunner {
	if isNilToolCatalog(toolCatalog) {
		registry := tools.NewRegistry()
		tools.RegisterBuiltins(registry)
		toolCatalog = registry
	}
	prompts := prompt.NewBuilder(workspace)
	runner := &ProviderRunner{
		tools:    toolCatalog,
		payloads: payloads,
	}
	runner.memoryExecutor = memoryRunExecutor{writer: runnercontext.NewMemoryWriter(workspace)}
	runner.llmExecutor = llmAgentExecutor{
		providers:       providers,
		contextBuilder:  runnercontext.NewAssembler(historyReader),
		promptAssembler: llmPromptAssembler{prompts: prompts, tools: runner.tools},
		toolExecutor:    llmToolExecutor{workspace: workspace, tools: runner.tools},
		traceAssembler:  runTraceAssembler{},
	}
	return runner
}

func isNilToolCatalog(toolCatalog kernel.ToolCatalog) bool {
	if toolCatalog == nil {
		return true
	}
	value := reflect.ValueOf(toolCatalog)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (r *ProviderRunner) Run(ctx context.Context, event model.InternalEvent, maxToolRounds int, sink StreamSink) (RunOutput, error) {
	start := time.Now().UTC()
	logger := runLogger(ctx, "runner", event)
	if event.Payload.Type == payloadTypeNewSession {
		logger.Info("new session shortcut", logging.String("run_mode", string(model.RunModeNormal)))
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
		logger.Error("payload registry unavailable")
		return RunOutput{}, errors.New("payload registry unavailable")
	}
	plan := r.payloads.Resolve(event.Payload.Type)
	planLogger := logger.With(
		logging.String("run_mode", string(plan.RunMode)),
		logging.String("execution_kind", string(plan.Kind)),
		logging.Bool("suppress_output", plan.SuppressOutput),
		logging.Bool("suppress_stream", plan.SuppressStream),
		logging.Int("allowed_tool_count", len(plan.AllowedTools)),
	)
	if plan.MemoryWriteTarget != "" {
		planLogger = planLogger.With(logging.String("memory_write_target", string(plan.MemoryWriteTarget)))
	}
	planLogger.Info("payload plan selected")
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
		return r.memoryExecutor.Execute(ctx, event, now, trace, plan)
	default:
		streamSink := sink
		if plan.SuppressStream {
			streamSink = nil
		}
		return r.llmExecutor.Execute(ctx, event, maxToolRounds, now, trace, newSafeStreamSink(streamSink, trace), llmRunOptions{
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

type llmRunOptions struct {
	runMode               model.RunMode
	suppressOutput        bool
	userVisible           bool
	toolVisible           bool
	finalAssistantVisible bool
	messageMeta           map[string]any
	allowedTools          map[string]struct{}
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
