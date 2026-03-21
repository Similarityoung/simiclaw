package runtime

import (
	"context"
	"strings"

	"github.com/similarityyoung/simiclaw/internal/runner"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type runnerExecutor struct {
	runner    runner.Runner
	maxRounds int
}

func NewRunnerExecutor(run runner.Runner, maxRounds int) kernel.Executor {
	if maxRounds <= 0 {
		maxRounds = 4
	}
	return runnerExecutor{
		runner:    run,
		maxRounds: maxRounds,
	}
}

func (e runnerExecutor) Execute(ctx context.Context, claim runtimemodel.ClaimContext, sink kernel.EventSink) (runtimemodel.ExecutionResult, error) {
	ctx = runner.WithRunID(ctx, claim.RunID)
	output, err := e.runner.Run(ctx, claim.Event, e.maxRounds, newRuntimeEventStreamSink(ctx, claim, sink))
	result, convErr := executionResultFromRunOutput(claim, output)
	if err == nil && convErr != nil {
		err = convErr
	}
	return result, err
}

func executionResultFromRunOutput(claim runtimemodel.ClaimContext, output runner.RunOutput) (runtimemodel.ExecutionResult, error) {
	result := runtimemodel.ExecutionResult{
		RunMode:           output.RunMode,
		AssistantReply:    output.AssistantReply,
		SuppressOutput:    output.SuppressOutput,
		ToolCalls:         output.Trace.ToolCalls,
		Diagnostics:       output.Trace.Diagnostics,
		Provider:          output.Trace.Provider,
		Model:             output.Trace.Model,
		PromptTokens:      output.Trace.PromptTokens,
		CompletionTokens:  output.Trace.CompletionTokens,
		TotalTokens:       output.Trace.TotalTokens,
		LatencyMS:         output.Trace.LatencyMS,
		FinishReason:      output.Trace.FinishReason,
		RawFinishReason:   output.Trace.RawFinishReason,
		ProviderRequestID: output.Trace.ProviderRequestID,
		OutputText:        output.Trace.OutputText,
	}
	for _, msg := range output.Messages {
		result.OutputMessages = append(result.OutputMessages, runtimemodel.StoredMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			Visible:    msg.Visible,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
			ToolName:   msg.ToolName,
			ToolArgs:   msg.ToolArgs,
			ToolResult: msg.ToolResult,
			Meta:       msg.Meta,
		})
	}
	if result.RunMode == "" {
		result.RunMode = claim.RunMode
	}
	if result.SuppressOutput || strings.TrimSpace(output.AssistantReply) == "" {
		return result, nil
	}
	intent, err := deliveryIntentForReply(claim.Event, output.AssistantReply)
	if err != nil {
		result.AssistantReply = ""
		return result, err
	}
	result.Delivery = intent
	return result, nil
}

func deliveryIntentForReply(event model.InternalEvent, reply string) (*runtimemodel.DeliveryIntent, error) {
	if strings.TrimSpace(reply) == "" {
		return nil, nil
	}
	intent := &runtimemodel.DeliveryIntent{Body: reply}
	if event.Source != "telegram" {
		return intent, nil
	}
	chatID, err := telegramTargetID(event)
	if err != nil {
		return nil, err
	}
	intent.Channel = "telegram"
	intent.TargetID = chatID
	return intent, nil
}

func newEventWorkItem(eventID string) runtimemodel.WorkItem {
	return runtimemodel.WorkItem{
		EventID: eventID,
	}
}
