package store

import (
	"encoding/json"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

const messageMetaToolCallsKey = "__tool_calls"

func encodeStoredMeta(meta map[string]any, toolCalls []model.ToolCall) (string, error) {
	if len(toolCalls) == 0 {
		b, err := json.Marshal(meta)
		return string(b), err
	}
	merged := make(map[string]any, len(meta)+1)
	for key, value := range meta {
		if key == messageMetaToolCallsKey {
			continue
		}
		merged[key] = value
	}
	merged[messageMetaToolCallsKey] = toolCalls
	b, err := json.Marshal(merged)
	return string(b), err
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
		if b, err := json.Marshal(rawToolCalls); err == nil {
			_ = json.Unmarshal(b, &toolCalls)
		}
		delete(meta, messageMetaToolCallsKey)
	}
	if len(meta) == 0 {
		meta = nil
	}
	return meta, toolCalls
}
