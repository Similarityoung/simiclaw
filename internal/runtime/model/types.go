package model

import (
	"time"

	pkgmodel "github.com/similarityyoung/simiclaw/pkg/model"
)

type ClaimedEvent struct {
	Event   pkgmodel.InternalEvent
	RunID   string
	Status  pkgmodel.EventStatus
	RunMode pkgmodel.RunMode
}

type StoredMessage struct {
	MessageID  string
	SessionKey string
	SessionID  string
	RunID      string
	Role       string
	Content    string
	Visible    bool
	ToolCalls  []pkgmodel.ToolCall
	ToolCallID string
	ToolName   string
	ToolArgs   map[string]any
	ToolResult map[string]any
	Meta       map[string]any
	CreatedAt  time.Time
}

type RunFinalize struct {
	RunID             string
	EventID           string
	SessionKey        string
	SessionID         string
	RunMode           pkgmodel.RunMode
	RunStatus         pkgmodel.RunStatus
	EventStatus       pkgmodel.EventStatus
	Provider          string
	Model             string
	PromptTokens      int
	CompletionTokens  int
	TotalTokens       int
	LatencyMS         int64
	FinishReason      string
	RawFinishReason   string
	ProviderRequestID string
	OutputText        string
	ToolCalls         []pkgmodel.ToolCall
	Diagnostics       map[string]string
	Error             *pkgmodel.ErrorBlock
	AssistantReply    string
	Messages          []StoredMessage
	OutboxChannel     string
	OutboxTargetID    string
	OutboxBody        string
	Now               time.Time
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
