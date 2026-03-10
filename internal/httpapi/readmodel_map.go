package httpapi

import (
	"github.com/similarityyoung/simiclaw/internal/readmodel"
	"github.com/similarityyoung/simiclaw/pkg/api"
)

func toAPIEventRecord(rec readmodel.EventRecord) api.EventRecord {
	return api.EventRecord{
		EventID:           rec.EventID,
		Status:            rec.Status,
		OutboxStatus:      rec.OutboxStatus,
		SessionKey:        rec.SessionKey,
		SessionID:         rec.SessionID,
		RunID:             rec.RunID,
		RunMode:           rec.RunMode,
		AssistantReply:    rec.AssistantReply,
		OutboxID:          rec.OutboxID,
		ProcessingLease:   rec.ProcessingLease,
		ReceivedAt:        rec.ReceivedAt,
		CreatedAt:         rec.CreatedAt,
		UpdatedAt:         rec.UpdatedAt,
		PayloadHash:       rec.PayloadHash,
		Provider:          rec.Provider,
		Model:             rec.Model,
		ProviderRequestID: rec.ProviderRequestID,
		Error:             rec.Error,
	}
}

func toAPISessionRecord(rec readmodel.SessionRecord) api.SessionRecord {
	return api.SessionRecord{
		SessionKey:            rec.SessionKey,
		ActiveSessionID:       rec.ActiveSessionID,
		ConversationID:        rec.ConversationID,
		ChannelType:           rec.ChannelType,
		ParticipantID:         rec.ParticipantID,
		DMScope:               rec.DMScope,
		MessageCount:          rec.MessageCount,
		PromptTokensTotal:     rec.PromptTokensTotal,
		CompletionTokensTotal: rec.CompletionTokensTotal,
		TotalTokensTotal:      rec.TotalTokensTotal,
		LastModel:             rec.LastModel,
		LastRunID:             rec.LastRunID,
		LastActivityAt:        rec.LastActivityAt,
		CreatedAt:             rec.CreatedAt,
		UpdatedAt:             rec.UpdatedAt,
	}
}

func toAPIMessageRecord(rec readmodel.MessageRecord) api.MessageRecord {
	return api.MessageRecord{
		MessageID:  rec.MessageID,
		SessionKey: rec.SessionKey,
		SessionID:  rec.SessionID,
		RunID:      rec.RunID,
		Role:       rec.Role,
		Content:    rec.Content,
		Visible:    rec.Visible,
		ToolCallID: rec.ToolCallID,
		ToolName:   rec.ToolName,
		ToolArgs:   rec.ToolArgs,
		ToolResult: rec.ToolResult,
		Meta:       rec.Meta,
		CreatedAt:  rec.CreatedAt,
	}
}
