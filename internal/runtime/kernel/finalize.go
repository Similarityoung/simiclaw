package kernel

import (
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (s *Service) buildFinalize(claim runtimemodel.ClaimContext, result runtimemodel.ExecutionResult, runErr error) runtimemodel.FinalizeCommand {
	now := s.now()
	finalize := runtimemodel.FinalizeCommand{
		RunID:             claim.RunID,
		EventID:           claim.Event.EventID,
		SessionKey:        claim.SessionKey,
		SessionID:         claim.SessionID,
		RunMode:           result.RunMode,
		RunStatus:         model.RunStatusCompleted,
		EventStatus:       model.EventStatusProcessed,
		Provider:          result.Provider,
		Model:             result.Model,
		PromptTokens:      result.PromptTokens,
		CompletionTokens:  result.CompletionTokens,
		TotalTokens:       result.TotalTokens,
		LatencyMS:         result.LatencyMS,
		FinishReason:      result.FinishReason,
		RawFinishReason:   result.RawFinishReason,
		ProviderRequestID: result.ProviderRequestID,
		OutputText:        result.OutputText,
		ToolCalls:         result.ToolCalls,
		Diagnostics:       result.Diagnostics,
		Messages:          s.storedMessages(now, claim, result.OutputMessages),
		Now:               now,
	}
	if finalize.RunMode == "" {
		finalize.RunMode = claim.RunMode
	}
	if runErr != nil {
		finalize.RunStatus = model.RunStatusFailed
		finalize.EventStatus = model.EventStatusFailed
		finalize.Error = &model.ErrorBlock{
			Code:    model.ErrorCodeInternal,
			Message: runErr.Error(),
		}
		finalize.AssistantReply = ""
		return finalize
	}
	if result.SuppressOutput {
		finalize.EventStatus = model.EventStatusSuppressed
		finalize.AssistantReply = ""
		return finalize
	}
	finalize.AssistantReply = result.AssistantReply
	if result.Delivery != nil {
		finalize.OutboxChannel = result.Delivery.Channel
		finalize.OutboxTargetID = result.Delivery.TargetID
		finalize.OutboxBody = result.Delivery.Body
	}
	return finalize
}

func (s *Service) storedMessages(now time.Time, claim runtimemodel.ClaimContext, messages []runtimemodel.StoredMessage) []runtimemodel.StoredMessage {
	if len(messages) == 0 {
		return nil
	}
	stored := make([]runtimemodel.StoredMessage, 0, len(messages))
	for _, msg := range messages {
		stored = append(stored, runtimemodel.StoredMessage{
			MessageID:  s.messageID(now),
			SessionKey: claim.SessionKey,
			SessionID:  claim.SessionID,
			RunID:      claim.RunID,
			Role:       msg.Role,
			Content:    msg.Content,
			Visible:    msg.Visible,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
			ToolName:   msg.ToolName,
			ToolArgs:   msg.ToolArgs,
			ToolResult: msg.ToolResult,
			Meta:       msg.Meta,
			CreatedAt:  now,
		})
	}
	return stored
}
