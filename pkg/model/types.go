package model

import (
	"encoding/json"
	"time"
)

type EventStatus string

const (
	EventStatusAccepted  EventStatus = "accepted"
	EventStatusRunning   EventStatus = "running"
	EventStatusCommitted EventStatus = "committed"
	EventStatusFailed    EventStatus = "failed"
)

type DeliveryStatus string

const (
	DeliveryStatusNotApplicable DeliveryStatus = "not_applicable"
	DeliveryStatusPending       DeliveryStatus = "pending"
	DeliveryStatusSent          DeliveryStatus = "sent"
	DeliveryStatusSuppressed    DeliveryStatus = "suppressed"
	DeliveryStatusFailed        DeliveryStatus = "failed"
)

type DeliveryDetail string

const (
	DeliveryDetailDirect        DeliveryDetail = "direct"
	DeliveryDetailSpooled       DeliveryDetail = "spooled"
	DeliveryDetailNotApplicable DeliveryDetail = "not_applicable"
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

type IngestRequest struct {
	Source         string       `json:"source"`
	Conversation   Conversation `json:"conversation"`
	SessionKeyHint string       `json:"session_key,omitempty"`
	IdempotencyKey string       `json:"idempotency_key"`
	Timestamp      string       `json:"timestamp"`
	Payload        EventPayload `json:"payload"`
}

type IngestResponse struct {
	EventID         string      `json:"event_id"`
	SessionKey      string      `json:"session_key"`
	ActiveSessionID string      `json:"active_session_id"`
	ReceivedAt      string      `json:"received_at"`
	PayloadHash     string      `json:"payload_hash"`
	Status          string      `json:"status"`
	StatusURL       string      `json:"status_url"`
	Error           *ErrorBlock `json:"error,omitempty"`
}

type ErrorResponse struct {
	Error ErrorBlock `json:"error"`
}

type ErrorBlock struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type EventRecord struct {
	EventID        string         `json:"event_id"`
	Status         EventStatus    `json:"status"`
	DeliveryStatus DeliveryStatus `json:"delivery_status"`
	DeliveryDetail DeliveryDetail `json:"delivery_detail"`
	OutboxID       string         `json:"outbox_id,omitempty"`
	SessionKey     string         `json:"session_key"`
	SessionID      string         `json:"session_id"`
	RunID          string         `json:"run_id,omitempty"`
	RunMode        RunMode        `json:"run_mode,omitempty"`
	CommitID       string         `json:"commit_id,omitempty"`
	AssistantReply string         `json:"assistant_reply,omitempty"`
	ReceivedAt     time.Time      `json:"received_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	PayloadHash    string         `json:"payload_hash"`
	Error          *ErrorBlock    `json:"error,omitempty"`
}

type SessionIndex struct {
	FormatVersion string                     `json:"format_version"`
	UpdatedAt     time.Time                  `json:"updated_at"`
	Sessions      map[string]SessionIndexRow `json:"sessions"`
}

type SessionIndexRow struct {
	ActiveSessionID string    `json:"active_session_id"`
	UpdatedAt       time.Time `json:"updated_at"`
	ConversationID  string    `json:"conversation_id,omitempty"`
	ChannelType     string    `json:"channel_type,omitempty"`
	ParticipantID   string    `json:"participant_id,omitempty"`
	DMScope         string    `json:"dm_scope,omitempty"`
	LastCommitID    string    `json:"last_commit_id,omitempty"`
	LastRunID       string    `json:"last_run_id,omitempty"`
}

type SessionHeader struct {
	Type          string    `json:"type"`
	SessionID     string    `json:"session_id"`
	SessionKey    string    `json:"session_key"`
	CreatedAt     time.Time `json:"created_at"`
	FormatVersion string    `json:"format_version"`
}

type SessionEntry struct {
	Type       string         `json:"type"`
	EntryID    string         `json:"entry_id"`
	RunID      string         `json:"run_id"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	OK         bool           `json:"ok,omitempty"`
	Result     string         `json:"result,omitempty"`
	Commit     *CommitMarker  `json:"commit,omitempty"`
	Meta       map[string]any `json:"meta,omitempty"`
}

type ToolCall struct {
	ToolCallID string         `json:"tool_call_id"`
	Name       string         `json:"name"`
	Args       map[string]any `json:"args,omitempty"`
}

type CommitMarker struct {
	CommitID    string `json:"commit_id"`
	RunID       string `json:"run_id"`
	EntryCount  int    `json:"entry_count"`
	LastEntryID string `json:"last_entry_id"`
}

type RunTrace struct {
	RunID           string            `json:"run_id"`
	EventID         string            `json:"event_id"`
	SessionKey      string            `json:"session_key"`
	SessionID       string            `json:"session_id"`
	RunMode         RunMode           `json:"run_mode"`
	ContextManifest *ContextManifest  `json:"context_manifest,omitempty"`
	RAGHits         []RAGHit          `json:"rag_hits,omitempty"`
	ToolExecutions  []ToolExecution   `json:"tool_executions,omitempty"`
	Actions         []Action          `json:"actions"`
	StartedAt       time.Time         `json:"started_at"`
	FinishedAt      time.Time         `json:"finished_at"`
	Error           *ErrorBlock       `json:"error,omitempty"`
	Diagnostics     map[string]string `json:"diagnostics,omitempty"`
}

type Action struct {
	ActionID             string         `json:"action_id"`
	ActionIndex          int            `json:"action_index"`
	ActionIdempotencyKey string         `json:"action_idempotency_key"`
	Type                 string         `json:"type"`
	Risk                 string         `json:"risk"`
	RequiresApproval     bool           `json:"requires_approval"`
	Payload              map[string]any `json:"payload"`
}

type ContextManifest struct {
	HistoryRange HistoryRange `json:"history_range"`
}

type HistoryRange struct {
	Mode           string `json:"mode"`
	CutoffCommitID string `json:"cutoff_commit_id,omitempty"`
	TailLimit      int    `json:"tail_limit,omitempty"`
}

type RAGHit struct {
	Path    string  `json:"path"`
	Scope   string  `json:"scope"`
	Lines   []int   `json:"lines"`
	Score   float64 `json:"score"`
	Preview string  `json:"preview"`
}

type ToolExecution struct {
	ToolCallID  string         `json:"tool_call_id"`
	Name        string         `json:"name"`
	Args        map[string]any `json:"args,omitempty"`
	ArgsSummary map[string]any `json:"args_summary,omitempty"`
	Result      map[string]any `json:"result,omitempty"`
	Error       *ErrorBlock    `json:"error,omitempty"`
}

type InboundLedgerRow struct {
	IdempotencyKey  string    `json:"idempotency_key"`
	EventID         string    `json:"event_id"`
	PayloadHash     string    `json:"payload_hash"`
	ReceivedAt      time.Time `json:"received_at"`
	SessionKey      string    `json:"session_key"`
	ActiveSessionID string    `json:"active_session_id"`
}

type OutboundLedgerRow struct {
	OutboundIdempotencyKey string    `json:"outbound_idempotency_key"`
	OutboxID               string    `json:"outbox_id"`
	CreatedAt              time.Time `json:"created_at"`
}

type OutboxMessage struct {
	OutboxID               string    `json:"outbox_id"`
	OutboundIdempotencyKey string    `json:"outbound_idempotency_key"`
	EventID                string    `json:"event_id"`
	SessionKey             string    `json:"session_key"`
	Body                   string    `json:"body"`
	CreatedAt              time.Time `json:"created_at"`
	Attempts               int       `json:"attempts"`
	LastError              string    `json:"last_error,omitempty"`
}

type Sessions struct {
	Items map[string][]SessionEntry `json:"items"`
}
