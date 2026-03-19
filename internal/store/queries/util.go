package queries

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

const messageMetaToolCallsKey = "__tool_calls"

func timeText(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func mustParseTime(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func normalizeListLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	return limit
}

func decodeStoredMeta(raw string) (map[string]any, []model.ToolCall) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" || trimmed == "{}" {
		return nil, nil
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return nil, nil
	}
	var toolCalls []model.ToolCall
	if rawToolCalls, ok := meta[messageMetaToolCallsKey]; ok {
		if encoded, err := json.Marshal(rawToolCalls); err == nil {
			_ = json.Unmarshal(encoded, &toolCalls)
		}
		delete(meta, messageMetaToolCallsKey)
	}
	if len(meta) == 0 {
		meta = nil
	}
	return meta, toolCalls
}

func reverseMessageRecords(items []MessageRecord) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func reverseHistory(items []HistoryMessage) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func shouldSkipPromptHistoryPayloadType(payloadType any) bool {
	value, _ := payloadType.(string)
	return value == "cron_fire" || value == "new_session"
}
