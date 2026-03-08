package model

import "time"

const ChatStreamProtocolVersion = "2026-03-07.sse.v1"

type ChatStreamEventType string

const (
	ChatStreamEventAccepted       ChatStreamEventType = "accepted"
	ChatStreamEventStatus         ChatStreamEventType = "status"
	ChatStreamEventReasoningDelta ChatStreamEventType = "reasoning_delta"
	ChatStreamEventTextDelta      ChatStreamEventType = "text_delta"
	ChatStreamEventToolStart      ChatStreamEventType = "tool_start"
	ChatStreamEventToolResult     ChatStreamEventType = "tool_result"
	ChatStreamEventDone           ChatStreamEventType = "done"
	ChatStreamEventError          ChatStreamEventType = "error"
)

type ChatStreamEvent struct {
	Type                  ChatStreamEventType `json:"type"`
	EventID               string              `json:"event_id"`
	Sequence              int64               `json:"sequence"`
	At                    time.Time           `json:"at"`
	StreamProtocolVersion string              `json:"stream_protocol_version,omitempty"`
	Status                string              `json:"status,omitempty"`
	Message               string              `json:"message,omitempty"`
	Delta                 string              `json:"delta,omitempty"`
	ToolCallID            string              `json:"tool_call_id,omitempty"`
	ToolName              string              `json:"tool_name,omitempty"`
	Args                  map[string]any      `json:"args,omitempty"`
	Result                map[string]any      `json:"result,omitempty"`
	Truncated             bool                `json:"truncated,omitempty"`
	IngestResponse        *IngestResponse     `json:"ingest_response,omitempty"`
	EventRecord           *EventRecord        `json:"event_record,omitempty"`
	Error                 *ErrorBlock         `json:"error,omitempty"`
}

func (e ChatStreamEvent) IsTerminal() bool {
	return e.Type == ChatStreamEventDone || e.Type == ChatStreamEventError
}
