package runner

import (
	"context"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/internal/provider"
	runnercontext "github.com/similarityyoung/simiclaw/internal/runner/context"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type llmAgentExecutor struct {
	providers       *provider.Factory
	contextBuilder  runnercontext.Assembler
	promptAssembler llmPromptAssembler
	toolExecutor    llmToolExecutor
	traceAssembler  runTraceAssembler
}

func (e llmAgentExecutor) Execute(ctx context.Context, event model.InternalEvent, maxToolRounds int, now time.Time, trace *api.RunTrace, sink StreamSink, opts llmRunOptions) (RunOutput, error) {
	defaultModel := e.providers.DefaultModel()
	logger := runLogger(ctx, "runner", event).With(logging.String("run_mode", string(opts.runMode)))
	providerLabel := providerName(defaultModel)
	llmProvider, actualModel, err := e.providers.Resolve(defaultModel)
	if err != nil {
		errFields := []logging.Field{logging.Error(err)}
		if providerLabel != "" {
			errFields = append(errFields, logging.String("provider", providerLabel))
		}
		logger.Error("provider resolve failed", errFields...)
		e.traceAssembler.Fail(trace, now, err)
		return RunOutput{RunMode: opts.runMode, Trace: *trace}, err
	}

	history, err := e.contextBuilder.Load(ctx, event.ActiveSessionID, event.Payload.Text)
	if err != nil {
		logger.Error("context load failed", logging.Error(err))
		e.traceAssembler.Fail(trace, now, err)
		return RunOutput{RunMode: opts.runMode, Trace: *trace}, err
	}
	e.traceAssembler.AttachContext(trace, history)

	messages := []OutputMessage{{
		Role:    "user",
		Content: strings.TrimSpace(event.Payload.Text),
		Visible: opts.userVisible,
		Meta:    cloneMap(opts.messageMeta),
	}}
	assembly := e.promptAssembler.Assemble(event, now, history.History, opts.allowedTools)
	chatMessages := assembly.chatMessages
	toolDefs := assembly.toolDefs

	var (
		totalUsage    provider.Usage
		reply         string
		last          provider.ChatResult
		toolUseCounts = map[string]int{}
		toolRounds    int
	)
	for round := 0; round <= maxToolRounds; round++ {
		callStartedAt := time.Now().UTC()
		providerLogger := logger.With(
			logging.String("provider", providerLabel),
			logging.String("model", actualModel),
			logging.Int("tool_round", round),
			logging.Int("message_count", len(chatMessages)),
			logging.Int("tool_definition_count", len(toolDefs)),
		)
		providerLogger.Info("provider started")
		last, err = llmProvider.StreamChat(ctx, provider.ChatRequest{
			Model:    actualModel,
			Messages: chatMessages,
			Tools:    toolDefs,
		}, providerStreamSink{sink: sink})
		if err != nil {
			providerLogger.Error(
				"provider failed",
				logging.String("error_kind", providerErrorKind(err)),
				logging.Int64("latency_ms", time.Since(callStartedAt).Milliseconds()),
				logging.Error(err),
			)
			e.traceAssembler.Fail(trace, now, err)
			return RunOutput{
				RunMode:  opts.runMode,
				Messages: messages,
				Trace:    *trace,
			}, err
		}
		providerFields := []logging.Field{
			logging.String("finish_reason", last.FinishReason),
			logging.Int("tool_call_count", len(last.ToolCalls)),
			logging.Int("prompt_tokens", last.Usage.PromptTokens),
			logging.Int("completion_tokens", last.Usage.CompletionTokens),
			logging.Int("total_tokens", last.Usage.TotalTokens),
			logging.Int64("latency_ms", time.Since(callStartedAt).Milliseconds()),
		}
		if last.ProviderRequestID != "" {
			providerFields = append(providerFields, logging.String("provider_request_id", last.ProviderRequestID))
		}
		providerLogger.Info("provider completed", providerFields...)
		totalUsage.PromptTokens += last.Usage.PromptTokens
		totalUsage.CompletionTokens += last.Usage.CompletionTokens
		totalUsage.TotalTokens += last.Usage.TotalTokens
		if len(last.ToolCalls) == 0 {
			reply = strings.TrimSpace(last.Text)
			break
		}
		if round == maxToolRounds {
			logger.Warn(
				"tool rounds exhausted",
				logging.String("provider", providerLabel),
				logging.String("model", actualModel),
				logging.Int("max_tool_rounds", maxToolRounds),
				logging.Int("tool_call_count", len(last.ToolCalls)),
			)
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
		toolRounds++
		messages = append(messages, assistantToolMessage)
		chatMessages = append(chatMessages, provider.ChatMessage{
			Role:      "assistant",
			Content:   strings.TrimSpace(last.Text),
			ToolCalls: cloneToolCalls(last.ToolCalls),
		})
		for _, call := range last.ToolCalls {
			step := e.toolExecutor.Execute(ctx, event, call, opts, toolUseCounts, sink, len(trace.Actions), now)
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

	e.traceAssembler.Complete(trace, now, totalUsage, last, reply)
	runFields := []logging.Field{
		logging.String("provider", last.Provider),
		logging.String("model", last.Model),
		logging.String("finish_reason", last.FinishReason),
		logging.Int("tool_rounds", toolRounds),
		logging.Int("prompt_tokens", totalUsage.PromptTokens),
		logging.Int("completion_tokens", totalUsage.CompletionTokens),
		logging.Int("total_tokens", totalUsage.TotalTokens),
		logging.Int64("latency_ms", trace.LatencyMS),
		logging.Bool("suppress_output", opts.suppressOutput),
	}
	if last.ProviderRequestID != "" {
		runFields = append(runFields, logging.String("provider_request_id", last.ProviderRequestID))
	}
	if opts.suppressOutput {
		logger.Info("run suppressed", runFields...)
	} else {
		logger.Info("run completed", runFields...)
	}

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
