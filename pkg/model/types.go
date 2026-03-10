package model

import (
	"encoding/json"
	"time"
)

type EventStatus string

const (
	EventStatusReceived   EventStatus = "received"
	EventStatusQueued     EventStatus = "queued"
	EventStatusProcessing EventStatus = "processing"
	EventStatusProcessed  EventStatus = "processed"
	EventStatusSuppressed EventStatus = "suppressed"
	EventStatusFailed     EventStatus = "failed"
)

type RunStatus string

const (
	RunStatusStarted   RunStatus = "started"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
)

type OutboxStatus string

const (
	OutboxStatusPending   OutboxStatus = "pending"
	OutboxStatusSending   OutboxStatus = "sending"
	OutboxStatusSent      OutboxStatus = "sent"
	OutboxStatusRetryWait OutboxStatus = "retry_wait"
	OutboxStatusDead      OutboxStatus = "dead"
)

type ScheduledJobKind string

const (
	ScheduledJobKindCron    ScheduledJobKind = "cron"
	ScheduledJobKindDelayed ScheduledJobKind = "delayed"
	ScheduledJobKindRetry   ScheduledJobKind = "retry"
)

type ScheduledJobStatus string

const (
	ScheduledJobStatusActive ScheduledJobStatus = "active"
	ScheduledJobStatusPaused ScheduledJobStatus = "paused"
)

type RunMode string

const (
	RunModeNormal  RunMode = "NORMAL"
	RunModeNoReply RunMode = "NO_REPLY"
)

type Conversation struct {
	ConversationID string `json:"conversation_id"`
	ThreadID       string `json:"thread_id,omitempty"`
	ChannelType    string `json:"channel_type"`
	ParticipantID  string `json:"participant_id,omitempty"`
}

type Attachment struct {
	AttachmentID string `json:"attachment_id,omitempty"`
	Filename     string `json:"filename,omitempty"`
	MIME         string `json:"mime,omitempty"`
	Size         int64  `json:"size,omitempty"`
}

type EventPayload struct {
	Type        string            `json:"type"`
	Text        string            `json:"text,omitempty"`
	Mentions    []string          `json:"mentions,omitempty"`
	Attachments []Attachment      `json:"attachments,omitempty"`
	Native      json.RawMessage   `json:"native,omitempty"`
	NativeRef   string            `json:"native_ref,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
}

type InternalEvent struct {
	EventID         string       `json:"event_id"`
	Source          string       `json:"source"`
	TenantID        string       `json:"tenant_id"`
	Scopes          []string     `json:"scopes,omitempty"`
	Conversation    Conversation `json:"conversation"`
	SessionKey      string       `json:"session_key"`
	IdempotencyKey  string       `json:"idempotency_key"`
	Timestamp       time.Time    `json:"timestamp"`
	Payload         EventPayload `json:"payload"`
	ActiveSessionID string       `json:"active_session_id"`
}

type ErrorBlock struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
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
	EventID           string       `json:"event_id"`
	Status            EventStatus  `json:"status"`
	OutboxStatus      OutboxStatus `json:"outbox_status,omitempty"`
	SessionKey        string       `json:"session_key"`
	SessionID         string       `json:"session_id"`
	RunID             string       `json:"run_id,omitempty"`
	RunMode           RunMode      `json:"run_mode,omitempty"`
	AssistantReply    string       `json:"assistant_reply,omitempty"`
	OutboxID          string       `json:"outbox_id,omitempty"`
	ProcessingLease   string       `json:"processing_started_at,omitempty"`
	ReceivedAt        time.Time    `json:"received_at"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
	PayloadHash       string       `json:"payload_hash"`
	Provider          string       `json:"provider,omitempty"`
	Model             string       `json:"model,omitempty"`
	ProviderRequestID string       `json:"provider_request_id,omitempty"`
	Error             *ErrorBlock  `json:"error,omitempty"`
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

type ToolCall struct {
	ToolCallID string         `json:"tool_call_id"`
	Name       string         `json:"name"`
	Args       map[string]any `json:"args,omitempty"`
}

type ToolExecution struct {
	ToolCallID  string         `json:"tool_call_id"`
	Name        string         `json:"name"`
	Args        map[string]any `json:"args,omitempty"`
	ArgsSummary map[string]any `json:"args_summary,omitempty"`
	Result      map[string]any `json:"result,omitempty"`
	Error       *ErrorBlock    `json:"error,omitempty"`
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
	RunMode           RunMode           `json:"run_mode"`
	Status            RunStatus         `json:"status"`
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
	ToolCalls         []ToolCall        `json:"tool_calls,omitempty"`
	Error             *ErrorBlock       `json:"error,omitempty"`
	Diagnostics       map[string]string `json:"diagnostics,omitempty"`
}

type OutboxMessage struct {
	OutboxID   string    `json:"outbox_id"`
	EventID    string    `json:"event_id"`
	SessionKey string    `json:"session_key"`
	Channel    string    `json:"channel,omitempty"`
	TargetID   string    `json:"target_id,omitempty"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
	Attempts   int       `json:"attempts"`
	LastError  string    `json:"last_error,omitempty"`
}
