package api

import (
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

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
	EventID           string             `json:"event_id"`
	Status            model.EventStatus  `json:"status"`
	OutboxStatus      model.OutboxStatus `json:"outbox_status,omitempty"`
	SessionKey        string             `json:"session_key"`
	SessionID         string             `json:"session_id"`
	RunID             string             `json:"run_id,omitempty"`
	RunMode           model.RunMode      `json:"run_mode,omitempty"`
	AssistantReply    string             `json:"assistant_reply,omitempty"`
	OutboxID          string             `json:"outbox_id,omitempty"`
	ProcessingLease   string             `json:"processing_started_at,omitempty"`
	ReceivedAt        time.Time          `json:"received_at"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
	PayloadHash       string             `json:"payload_hash"`
	Provider          string             `json:"provider,omitempty"`
	Model             string             `json:"model,omitempty"`
	ProviderRequestID string             `json:"provider_request_id,omitempty"`
	Error             *model.ErrorBlock  `json:"error,omitempty"`
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
