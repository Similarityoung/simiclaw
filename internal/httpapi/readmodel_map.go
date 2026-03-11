package httpapi

import (
	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
	"github.com/similarityyoung/simiclaw/pkg/api"
)

func toAPIEventRecord(rec querymodel.EventRecord) api.EventRecord {
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

func toAPISessionRecord(rec querymodel.SessionRecord) api.SessionRecord {
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

func toAPIMessageRecord(rec querymodel.MessageRecord) api.MessageRecord {
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

func toAPIRunTrace(trace querymodel.RunTrace) api.RunTrace {
	return api.RunTrace{
		RunID:             trace.RunID,
		EventID:           trace.EventID,
		SessionKey:        trace.SessionKey,
		SessionID:         trace.SessionID,
		RunMode:           trace.RunMode,
		Status:            trace.Status,
		ContextManifest:   toAPIContextManifest(trace.ContextManifest),
		RAGHits:           toAPIRAGHits(trace.RAGHits),
		ToolExecutions:    toAPIToolExecutions(trace.ToolExecutions),
		Actions:           toAPIActions(trace.Actions),
		StartedAt:         trace.StartedAt,
		FinishedAt:        trace.FinishedAt,
		Provider:          trace.Provider,
		Model:             trace.Model,
		PromptTokens:      trace.PromptTokens,
		CompletionTokens:  trace.CompletionTokens,
		TotalTokens:       trace.TotalTokens,
		LatencyMS:         trace.LatencyMS,
		FinishReason:      trace.FinishReason,
		RawFinishReason:   trace.RawFinishReason,
		ProviderRequestID: trace.ProviderRequestID,
		OutputText:        trace.OutputText,
		ToolCalls:         trace.ToolCalls,
		Error:             trace.Error,
		Diagnostics:       trace.Diagnostics,
	}
}

func toAPIContextManifest(in *querymodel.ContextManifest) *api.ContextManifest {
	if in == nil {
		return nil
	}
	return &api.ContextManifest{
		HistoryRange: api.HistoryRange{
			Mode:      in.HistoryRange.Mode,
			TailLimit: in.HistoryRange.TailLimit,
		},
	}
}

func toAPIRAGHits(in []querymodel.RAGHit) []api.RAGHit {
	if in == nil {
		return nil
	}
	out := make([]api.RAGHit, 0, len(in))
	for _, hit := range in {
		out = append(out, api.RAGHit{
			Path:    hit.Path,
			Scope:   hit.Scope,
			Lines:   hit.Lines,
			Score:   hit.Score,
			Preview: hit.Preview,
		})
	}
	return out
}

func toAPIToolExecutions(in []querymodel.ToolExecution) []api.ToolExecution {
	if in == nil {
		return nil
	}
	out := make([]api.ToolExecution, 0, len(in))
	for _, exec := range in {
		out = append(out, api.ToolExecution{
			ToolCallID:  exec.ToolCallID,
			Name:        exec.Name,
			Args:        exec.Args,
			ArgsSummary: exec.ArgsSummary,
			Result:      exec.Result,
			Error:       exec.Error,
		})
	}
	return out
}

func toAPIActions(in []querymodel.Action) []api.Action {
	if in == nil {
		return nil
	}
	out := make([]api.Action, 0, len(in))
	for _, action := range in {
		out = append(out, api.Action{
			ActionID:             action.ActionID,
			ActionIndex:          action.ActionIndex,
			ActionIdempotencyKey: action.ActionIdempotencyKey,
			Type:                 action.Type,
			Risk:                 action.Risk,
			RequiresApproval:     action.RequiresApproval,
			Payload:              action.Payload,
		})
	}
	return out
}
