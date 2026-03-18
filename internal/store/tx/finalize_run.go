package tx

import (
	"context"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/internal/store"
)

func (r *RuntimeRepository) Finalize(ctx context.Context, cmd runtimemodel.FinalizeCommand) error {
	return r.db.FinalizeRun(ctx, toStoreRunFinalize(cmd))
}

func toStoreRunFinalize(in runtimemodel.RunFinalize) store.RunFinalize {
	out := store.RunFinalize{
		RunID:             in.RunID,
		EventID:           in.EventID,
		SessionKey:        in.SessionKey,
		SessionID:         in.SessionID,
		RunMode:           in.RunMode,
		RunStatus:         in.RunStatus,
		EventStatus:       in.EventStatus,
		Provider:          in.Provider,
		Model:             in.Model,
		PromptTokens:      in.PromptTokens,
		CompletionTokens:  in.CompletionTokens,
		TotalTokens:       in.TotalTokens,
		LatencyMS:         in.LatencyMS,
		FinishReason:      in.FinishReason,
		RawFinishReason:   in.RawFinishReason,
		ProviderRequestID: in.ProviderRequestID,
		OutputText:        in.OutputText,
		ToolCalls:         in.ToolCalls,
		Diagnostics:       in.Diagnostics,
		Error:             in.Error,
		AssistantReply:    in.AssistantReply,
		OutboxChannel:     in.OutboxChannel,
		OutboxTargetID:    in.OutboxTargetID,
		OutboxBody:        in.OutboxBody,
		Now:               in.Now,
	}
	out.Messages = make([]store.StoredMessage, 0, len(in.Messages))
	for _, msg := range in.Messages {
		out.Messages = append(out.Messages, store.StoredMessage{
			MessageID:  msg.MessageID,
			SessionKey: msg.SessionKey,
			SessionID:  msg.SessionID,
			RunID:      msg.RunID,
			Role:       msg.Role,
			Content:    msg.Content,
			Visible:    msg.Visible,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
			ToolName:   msg.ToolName,
			ToolArgs:   msg.ToolArgs,
			ToolResult: msg.ToolResult,
			Meta:       msg.Meta,
			CreatedAt:  msg.CreatedAt,
		})
	}
	return out
}
