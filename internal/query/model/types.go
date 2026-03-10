package model

import (
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"
	pkgmodel "github.com/similarityyoung/simiclaw/pkg/model"
)

type EventCursorAnchor struct {
	CreatedAt time.Time
	EventID   string
}

type RunCursorAnchor struct {
	StartedAt time.Time
	RunID     string
}

type SessionCursorAnchor struct {
	LastActivityAt time.Time
	SessionKey     string
}

type MessageCursorAnchor struct {
	CreatedAt time.Time
	MessageID string
}

type EventFilter struct {
	SessionKey string
	Status     pkgmodel.EventStatus
	Limit      int
	Cursor     *EventCursorAnchor
}

type RunFilter struct {
	SessionKey string
	SessionID  string
	Limit      int
	Cursor     *RunCursorAnchor
}

type SessionFilter struct {
	SessionKey     string
	ConversationID string
	Limit          int
	Cursor         *SessionCursorAnchor
}

type SessionHistoryFilter struct {
	SessionID   string
	VisibleOnly bool
	Limit       int
	Cursor      *MessageCursorAnchor
}

type EventPage struct {
	Items []EventRecord
	Next  *EventCursorAnchor
}

type RunPage struct {
	Items []RunTrace
	Next  *RunCursorAnchor
}

type SessionPage struct {
	Items []SessionRecord
	Next  *SessionCursorAnchor
}

type MessagePage struct {
	Items []MessageRecord
	Next  *MessageCursorAnchor
}

type MessageRecord struct {
	MessageID  string         `json:"message_id"`
	SessionKey string         `json:"session_key"`
	SessionID  string         `json:"session_id"`
	RunID      string         `json:"run_id"`
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	Visible    bool           `json:"visible"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	ToolArgs   map[string]any `json:"tool_args,omitempty"`
	ToolResult map[string]any `json:"tool_result,omitempty"`
	Meta       map[string]any `json:"meta,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

type EventRecord struct {
	EventID           string                `json:"event_id"`
	Status            pkgmodel.EventStatus  `json:"status"`
	OutboxStatus      pkgmodel.OutboxStatus `json:"outbox_status,omitempty"`
	SessionKey        string                `json:"session_key"`
	SessionID         string                `json:"session_id"`
	RunID             string                `json:"run_id,omitempty"`
	RunMode           pkgmodel.RunMode      `json:"run_mode,omitempty"`
	AssistantReply    string                `json:"assistant_reply,omitempty"`
	OutboxID          string                `json:"outbox_id,omitempty"`
	ProcessingLease   string                `json:"processing_started_at,omitempty"`
	ReceivedAt        time.Time             `json:"received_at"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
	PayloadHash       string                `json:"payload_hash"`
	Provider          string                `json:"provider,omitempty"`
	Model             string                `json:"model,omitempty"`
	ProviderRequestID string                `json:"provider_request_id,omitempty"`
	Error             *pkgmodel.ErrorBlock  `json:"error,omitempty"`
}

type SessionRecord struct {
	SessionKey            string    `json:"session_key"`
	ActiveSessionID       string    `json:"active_session_id"`
	ConversationID        string    `json:"conversation_id,omitempty"`
	ChannelType           string    `json:"channel_type,omitempty"`
	ParticipantID         string    `json:"participant_id,omitempty"`
	DMScope               string    `json:"dm_scope,omitempty"`
	MessageCount          int       `json:"message_count"`
	PromptTokensTotal     int       `json:"prompt_tokens_total"`
	CompletionTokensTotal int       `json:"completion_tokens_total"`
	TotalTokensTotal      int       `json:"total_tokens_total"`
	LastModel             string    `json:"last_model,omitempty"`
	LastRunID             string    `json:"last_run_id,omitempty"`
	LastActivityAt        time.Time `json:"last_activity_at"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type LookupEvent struct {
	EventID     string    `json:"event_id"`
	PayloadHash string    `json:"payload_hash"`
	ReceivedAt  time.Time `json:"received_at"`
	SessionKey  string    `json:"session_key"`
	SessionID   string    `json:"session_id"`
}

type RunTrace struct {
	RunID             string               `json:"run_id"`
	EventID           string               `json:"event_id"`
	SessionKey        string               `json:"session_key"`
	SessionID         string               `json:"session_id"`
	RunMode           pkgmodel.RunMode     `json:"run_mode"`
	Status            pkgmodel.RunStatus   `json:"status"`
	ContextManifest   *api.ContextManifest `json:"context_manifest,omitempty"`
	RAGHits           []api.RAGHit         `json:"rag_hits,omitempty"`
	ToolExecutions    []api.ToolExecution  `json:"tool_executions,omitempty"`
	Actions           []api.Action         `json:"actions,omitempty"`
	StartedAt         time.Time            `json:"started_at"`
	FinishedAt        time.Time            `json:"finished_at"`
	Provider          string               `json:"provider,omitempty"`
	Model             string               `json:"model,omitempty"`
	PromptTokens      int                  `json:"prompt_tokens,omitempty"`
	CompletionTokens  int                  `json:"completion_tokens,omitempty"`
	TotalTokens       int                  `json:"total_tokens,omitempty"`
	LatencyMS         int64                `json:"latency_ms,omitempty"`
	FinishReason      string               `json:"finish_reason,omitempty"`
	RawFinishReason   string               `json:"raw_finish_reason,omitempty"`
	ProviderRequestID string               `json:"provider_request_id,omitempty"`
	OutputText        string               `json:"output_text,omitempty"`
	ToolCalls         []pkgmodel.ToolCall  `json:"tool_calls,omitempty"`
	Error             *pkgmodel.ErrorBlock `json:"error,omitempty"`
	Diagnostics       map[string]string    `json:"diagnostics,omitempty"`
}
