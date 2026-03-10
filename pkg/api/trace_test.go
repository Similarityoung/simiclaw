package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type legacyToolExecution struct {
	ToolCallID  string            `json:"tool_call_id"`
	Name        string            `json:"name"`
	Args        map[string]any    `json:"args,omitempty"`
	ArgsSummary map[string]any    `json:"args_summary,omitempty"`
	Result      map[string]any    `json:"result,omitempty"`
	Error       *model.ErrorBlock `json:"error,omitempty"`
}

type legacyAction struct {
	ActionID             string         `json:"action_id"`
	ActionIndex          int            `json:"action_index"`
	ActionIdempotencyKey string         `json:"action_idempotency_key"`
	Type                 string         `json:"type"`
	Risk                 string         `json:"risk"`
	RequiresApproval     bool           `json:"requires_approval"`
	Payload              map[string]any `json:"payload,omitempty"`
}

type legacyHistoryRange struct {
	Mode      string `json:"mode"`
	TailLimit int    `json:"tail_limit,omitempty"`
}

type legacyContextManifest struct {
	HistoryRange legacyHistoryRange `json:"history_range"`
}

type legacyRAGHit struct {
	Path    string  `json:"path"`
	Scope   string  `json:"scope"`
	Lines   []int   `json:"lines"`
	Score   float64 `json:"score"`
	Preview string  `json:"preview"`
}

type legacyRunTrace struct {
	RunID             string                 `json:"run_id"`
	EventID           string                 `json:"event_id"`
	SessionKey        string                 `json:"session_key"`
	SessionID         string                 `json:"session_id"`
	RunMode           model.RunMode          `json:"run_mode"`
	Status            model.RunStatus        `json:"status"`
	ContextManifest   *legacyContextManifest `json:"context_manifest,omitempty"`
	RAGHits           []legacyRAGHit         `json:"rag_hits,omitempty"`
	ToolExecutions    []legacyToolExecution  `json:"tool_executions,omitempty"`
	Actions           []legacyAction         `json:"actions,omitempty"`
	StartedAt         time.Time              `json:"started_at"`
	FinishedAt        time.Time              `json:"finished_at"`
	Provider          string                 `json:"provider,omitempty"`
	Model             string                 `json:"model,omitempty"`
	PromptTokens      int                    `json:"prompt_tokens,omitempty"`
	CompletionTokens  int                    `json:"completion_tokens,omitempty"`
	TotalTokens       int                    `json:"total_tokens,omitempty"`
	LatencyMS         int64                  `json:"latency_ms,omitempty"`
	FinishReason      string                 `json:"finish_reason,omitempty"`
	RawFinishReason   string                 `json:"raw_finish_reason,omitempty"`
	ProviderRequestID string                 `json:"provider_request_id,omitempty"`
	OutputText        string                 `json:"output_text,omitempty"`
	ToolCalls         []model.ToolCall       `json:"tool_calls,omitempty"`
	Error             *model.ErrorBlock      `json:"error,omitempty"`
	Diagnostics       map[string]string      `json:"diagnostics,omitempty"`
}

func TestRunTraceJSONCompatibilityWithLegacyModel(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	legacy := legacyRunTrace{
		RunID:      "run-1",
		EventID:    "evt-1",
		SessionKey: "local:dm:u1",
		SessionID:  "ses-1",
		RunMode:    model.RunModeNormal,
		Status:     model.RunStatusCompleted,
		ContextManifest: &legacyContextManifest{
			HistoryRange: legacyHistoryRange{Mode: "tail", TailLimit: 20},
		},
		RAGHits: []legacyRAGHit{{
			Path:    "workspace/memory/day.md",
			Scope:   "session",
			Lines:   []int{3, 4},
			Score:   0.75,
			Preview: "hello memory",
		}},
		ToolExecutions: []legacyToolExecution{{
			ToolCallID:  "call-1",
			Name:        "memory_search",
			Args:        map[string]any{"query": "hello"},
			ArgsSummary: map[string]any{"query": "hello"},
			Result:      map[string]any{"hits": 1},
			Error:       &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: "boom"},
		}},
		Actions: []legacyAction{{
			ActionID:             "act-1",
			ActionIndex:          1,
			ActionIdempotencyKey: "idem-1",
			Type:                 "tool",
			Risk:                 "low",
			RequiresApproval:     false,
			Payload:              map[string]any{"name": "memory_search"},
		}},
		StartedAt:         now,
		FinishedAt:        now.Add(2 * time.Second),
		Provider:          "fake",
		Model:             "default",
		PromptTokens:      11,
		CompletionTokens:  13,
		TotalTokens:       24,
		LatencyMS:         2000,
		FinishReason:      "stop",
		RawFinishReason:   "stop",
		ProviderRequestID: "req-1",
		OutputText:        "done",
		ToolCalls: []model.ToolCall{{
			ToolCallID: "call-1",
			Name:       "memory_search",
			Args:       map[string]any{"query": "hello"},
		}},
		Error:       &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: "boom"},
		Diagnostics: map[string]string{"a": "b"},
	}

	current := RunTrace{
		RunID:      legacy.RunID,
		EventID:    legacy.EventID,
		SessionKey: legacy.SessionKey,
		SessionID:  legacy.SessionID,
		RunMode:    legacy.RunMode,
		Status:     legacy.Status,
		ContextManifest: &ContextManifest{
			HistoryRange: HistoryRange{
				Mode:      legacy.ContextManifest.HistoryRange.Mode,
				TailLimit: legacy.ContextManifest.HistoryRange.TailLimit,
			},
		},
		RAGHits: []RAGHit{{
			Path:    legacy.RAGHits[0].Path,
			Scope:   legacy.RAGHits[0].Scope,
			Lines:   legacy.RAGHits[0].Lines,
			Score:   legacy.RAGHits[0].Score,
			Preview: legacy.RAGHits[0].Preview,
		}},
		ToolExecutions: []ToolExecution{{
			ToolCallID:  legacy.ToolExecutions[0].ToolCallID,
			Name:        legacy.ToolExecutions[0].Name,
			Args:        legacy.ToolExecutions[0].Args,
			ArgsSummary: legacy.ToolExecutions[0].ArgsSummary,
			Result:      legacy.ToolExecutions[0].Result,
			Error:       legacy.ToolExecutions[0].Error,
		}},
		Actions: []Action{{
			ActionID:             legacy.Actions[0].ActionID,
			ActionIndex:          legacy.Actions[0].ActionIndex,
			ActionIdempotencyKey: legacy.Actions[0].ActionIdempotencyKey,
			Type:                 legacy.Actions[0].Type,
			Risk:                 legacy.Actions[0].Risk,
			RequiresApproval:     legacy.Actions[0].RequiresApproval,
			Payload:              legacy.Actions[0].Payload,
		}},
		StartedAt:         legacy.StartedAt,
		FinishedAt:        legacy.FinishedAt,
		Provider:          legacy.Provider,
		Model:             legacy.Model,
		PromptTokens:      legacy.PromptTokens,
		CompletionTokens:  legacy.CompletionTokens,
		TotalTokens:       legacy.TotalTokens,
		LatencyMS:         legacy.LatencyMS,
		FinishReason:      legacy.FinishReason,
		RawFinishReason:   legacy.RawFinishReason,
		ProviderRequestID: legacy.ProviderRequestID,
		OutputText:        legacy.OutputText,
		ToolCalls:         legacy.ToolCalls,
		Error:             legacy.Error,
		Diagnostics:       legacy.Diagnostics,
	}

	legacyJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy trace: %v", err)
	}
	currentJSON, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("marshal current trace: %v", err)
	}
	if string(currentJSON) != string(legacyJSON) {
		t.Fatalf("unexpected trace json:\nlegacy: %s\ncurrent: %s", string(legacyJSON), string(currentJSON))
	}
}
