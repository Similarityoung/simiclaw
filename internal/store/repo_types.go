package store

import (
	"errors"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

var ErrIdempotencyConflict = errors.New("idempotency payload hash mismatch")

type IngestResult struct {
	EventID         string
	SessionKey      string
	SessionID       string
	ReceivedAt      time.Time
	PayloadHash     string
	Duplicate       bool
	ExistingEventID string
}

type StoredMessage struct {
	MessageID  string
	SessionKey string
	SessionID  string
	RunID      string
	Role       string
	Content    string
	Visible    bool
	ToolCalls  []model.ToolCall
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
	RunMode           model.RunMode
	RunStatus         model.RunStatus
	EventStatus       model.EventStatus
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
	ToolCalls         []model.ToolCall
	Diagnostics       map[string]string
	Error             *model.ErrorBlock
	AssistantReply    string
	Messages          []StoredMessage
	OutboxChannel     string
	OutboxTargetID    string
	OutboxBody        string
	Now               time.Time
}

type ClaimedEvent struct {
	Event   model.InternalEvent
	RunID   string
	Status  model.EventStatus
	RunMode model.RunMode
}

type HistoryMessage struct {
	Role       string
	Content    string
	ToolCalls  []model.ToolCall
	ToolCallID string
	ToolName   string
	Meta       map[string]any
}

type ClaimedOutbox struct {
	OutboxID     string
	EventID      string
	SessionKey   string
	Channel      string
	TargetID     string
	Body         string
	AttemptCount int
	CreatedAt    time.Time
}
