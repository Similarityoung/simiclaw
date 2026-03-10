package api

import (
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type ToolExecution struct {
	ToolCallID  string            `json:"tool_call_id"`
	Name        string            `json:"name"`
	Args        map[string]any    `json:"args,omitempty"`
	ArgsSummary map[string]any    `json:"args_summary,omitempty"`
	Result      map[string]any    `json:"result,omitempty"`
	Error       *model.ErrorBlock `json:"error,omitempty"`
}

type Action struct {
	ActionID             string         `json:"action_id"`
	ActionIndex          int            `json:"action_index"`
	ActionIdempotencyKey string         `json:"action_idempotency_key"`
	Type                 string         `json:"type"`
	Risk                 string         `json:"risk"`
	RequiresApproval     bool           `json:"requires_approval"`
	Payload              map[string]any `json:"payload,omitempty"`
}

type HistoryRange struct {
	Mode      string `json:"mode"`
	TailLimit int    `json:"tail_limit,omitempty"`
}

type ContextManifest struct {
	HistoryRange HistoryRange `json:"history_range"`
}

type RAGHit struct {
	Path    string  `json:"path"`
	Scope   string  `json:"scope"`
	Lines   []int   `json:"lines"`
	Score   float64 `json:"score"`
	Preview string  `json:"preview"`
}

type RunTrace struct {
	RunID             string            `json:"run_id"`
	EventID           string            `json:"event_id"`
	SessionKey        string            `json:"session_key"`
	SessionID         string            `json:"session_id"`
	RunMode           model.RunMode     `json:"run_mode"`
	Status            model.RunStatus   `json:"status"`
	ContextManifest   *ContextManifest  `json:"context_manifest,omitempty"`
	RAGHits           []RAGHit          `json:"rag_hits,omitempty"`
	ToolExecutions    []ToolExecution   `json:"tool_executions,omitempty"`
	Actions           []Action          `json:"actions,omitempty"`
	StartedAt         time.Time         `json:"started_at"`
	FinishedAt        time.Time         `json:"finished_at"`
	Provider          string            `json:"provider,omitempty"`
	Model             string            `json:"model,omitempty"`
	PromptTokens      int               `json:"prompt_tokens,omitempty"`
	CompletionTokens  int               `json:"completion_tokens,omitempty"`
	TotalTokens       int               `json:"total_tokens,omitempty"`
	LatencyMS         int64             `json:"latency_ms,omitempty"`
	FinishReason      string            `json:"finish_reason,omitempty"`
	RawFinishReason   string            `json:"raw_finish_reason,omitempty"`
	ProviderRequestID string            `json:"provider_request_id,omitempty"`
	OutputText        string            `json:"output_text,omitempty"`
	ToolCalls         []model.ToolCall  `json:"tool_calls,omitempty"`
	Error             *model.ErrorBlock `json:"error,omitempty"`
	Diagnostics       map[string]string `json:"diagnostics,omitempty"`
}
