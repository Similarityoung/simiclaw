package runtime

import (
	"context"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/internal/runner"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/internal/streaming"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type eventLoopFactsAdapter struct {
	repo EventLoopRepository
}

func (a eventLoopFactsAdapter) ListRunnable(ctx context.Context, limit int) ([]runtimemodel.WorkItem, error) {
	ids, err := a.repo.ListRunnableEventIDs(ctx, limit)
	if err != nil {
		return nil, err
	}
	items := make([]runtimemodel.WorkItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, newEventWorkItem(id))
	}
	return items, nil
}

func (a eventLoopFactsAdapter) ClaimWork(ctx context.Context, work runtimemodel.WorkItem, runID string, now time.Time) (runtimemodel.ClaimContext, bool, error) {
	eventID := work.EventID
	if eventID == "" {
		eventID = work.Identity
	}
	claimed, ok, err := a.repo.ClaimLoopEvent(ctx, eventID, runID, now)
	if err != nil || !ok {
		return runtimemodel.ClaimContext{}, ok, err
	}
	channel := ""
	if claimed.Event.Source == "telegram" {
		channel = "telegram"
	}
	return runtimemodel.ClaimContext{
		Work:       newEventWorkItem(claimed.Event.EventID),
		Event:      claimed.Event,
		RunID:      claimed.RunID,
		RunMode:    claimed.RunMode,
		SessionKey: claimed.Event.SessionKey,
		SessionID:  claimed.Event.ActiveSessionID,
		Source:     claimed.Event.Source,
		Channel:    channel,
	}, true, nil
}

func (a eventLoopFactsAdapter) Finalize(ctx context.Context, cmd runtimemodel.FinalizeCommand) error {
	return a.repo.FinalizeLoopRun(ctx, cmd)
}

func (a eventLoopFactsAdapter) GetEventRecord(ctx context.Context, eventID string) (runtimemodel.EventRecord, bool, error) {
	return a.repo.GetLoopEventRecord(ctx, eventID)
}

type runnerExecutor struct {
	runner    runner.Runner
	maxRounds int
	streamHub *streaming.Hub
}

func (e runnerExecutor) Execute(ctx context.Context, claim runtimemodel.ClaimContext, _ kernel.EventSink) (runtimemodel.ExecutionResult, error) {
	output, err := e.runner.Run(ctx, claim.Event, e.maxRounds, newHubStreamSink(e.streamHub, claim.Event.EventID))
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
		Kind:     runtimemodel.WorkKindEvent,
		Identity: eventID,
		EventID:  eventID,
	}
}
