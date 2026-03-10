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

type ToolCall struct {
	ToolCallID string         `json:"tool_call_id"`
	Name       string         `json:"name"`
	Args       map[string]any `json:"args,omitempty"`
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
