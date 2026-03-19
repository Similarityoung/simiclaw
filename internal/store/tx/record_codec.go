package tx

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

var errNoMutation = errors.New("no-op")

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

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func isNoReplyPayload(payloadType string) bool {
	return payloadType == "memory_flush" || payloadType == "compaction" || payloadType == "cron_fire"
}

func encodeStoredMeta(meta map[string]any, toolCalls []model.ToolCall) (string, error) {
	if len(toolCalls) == 0 {
		encoded, err := json.Marshal(meta)
		return string(encoded), err
	}
	merged := make(map[string]any, len(meta)+1)
	for key, value := range meta {
		if key == messageMetaToolCallsKey {
			continue
		}
		merged[key] = value
	}
	merged[messageMetaToolCallsKey] = toolCalls
	encoded, err := json.Marshal(merged)
	return string(encoded), err
}
