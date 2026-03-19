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

type WorkKind string

const (
	WorkKindEvent        WorkKind = "event"
	WorkKindOutbox       WorkKind = "outbox"
	WorkKindScheduledJob WorkKind = "scheduled_job"
	WorkKindRecovery     WorkKind = "recovery"
)

type WorkItem struct {
	Kind       WorkKind
	Identity   string
	EventID    string
	OutboxID   string
	JobID      string
	SessionKey string
	LaneKey    string
	Source     string
	Channel    string
	Metadata   map[string]string
}

type ClaimContext struct {
	Work       WorkItem
	Event      pkgmodel.InternalEvent
	RunID      string
	RunMode    pkgmodel.RunMode
	SessionKey string
	SessionID  string
	Source     string
	Channel    string
	Metadata   map[string]string
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

type FinalizeCommand = RunFinalize

type DeliveryIntent struct {
	Channel  string
	TargetID string
	Body     string
	Metadata map[string]string
}

type ExecutionResult struct {
	RunMode           pkgmodel.RunMode
	AssistantReply    string
	OutputMessages    []StoredMessage
	ToolCalls         []pkgmodel.ToolCall
	Diagnostics       map[string]string
	Delivery          *DeliveryIntent
	SuppressOutput    bool
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
	Error             *pkgmodel.ErrorBlock
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

type ScheduledJobPayload struct {
	Source          string
	TenantID        string
	Conversation    pkgmodel.Conversation
	Payload         pkgmodel.EventPayload
	IntervalSeconds int64
}

type ClaimedJob struct {
	JobID        string
	Name         string
	Kind         pkgmodel.ScheduledJobKind
	Status       pkgmodel.ScheduledJobStatus
	Payload      ScheduledJobPayload
	AttemptCount int
	NextRunAt    time.Time
}

type RuntimeEventKind string

const (
	RuntimeEventClaimed         RuntimeEventKind = "claimed"
	RuntimeEventExecuting       RuntimeEventKind = "executing"
	RuntimeEventReasoningDelta  RuntimeEventKind = "reasoning_delta"
	RuntimeEventTextDelta       RuntimeEventKind = "text_delta"
	RuntimeEventToolStarted     RuntimeEventKind = "tool_started"
	RuntimeEventToolFinished    RuntimeEventKind = "tool_finished"
	RuntimeEventFinalizeStarted RuntimeEventKind = "finalize_started"
	RuntimeEventCompleted       RuntimeEventKind = "completed"
	RuntimeEventFailed          RuntimeEventKind = "failed"
)

type RuntimeEvent struct {
	Kind        RuntimeEventKind
	Work        WorkItem
	EventID     string
	Sequence    int64
	RunID       string
	SessionKey  string
	SessionID   string
	Status      string
	Delta       string
	ToolCallID  string
	ToolName    string
	Args        map[string]any
	Result      map[string]any
	Truncated   bool
	Message     string
	OccurredAt  time.Time
	Metadata    map[string]string
	Error       *pkgmodel.ErrorBlock
	EventRecord *EventRecord
}

func (e RuntimeEvent) IsTerminal() bool {
	return e.Kind == RuntimeEventCompleted || e.Kind == RuntimeEventFailed
}
