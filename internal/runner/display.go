package runner

import (
	"encoding/json"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

const (
	maxDisplayDepth    = 4
	maxDisplayItems    = 20
	maxDisplayRuneSize = 256
)

func sanitizeDisplayMap(input map[string]any) (map[string]any, bool) {
	if len(input) == 0 {
		return map[string]any{}, false
	}
	truncated := false
	out := sanitizeDisplayValue(input, 0, &truncated, "").(map[string]any)
	return out, truncated
}

func sanitizeDisplayValue(input any, depth int, truncated *bool, key string) any {
	if depth >= maxDisplayDepth {
		*truncated = true
		return "[truncated]"
	}
	switch value := input.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		count := 0
		for childKey, childValue := range value {
			if count >= maxDisplayItems {
				out["_truncated_items"] = true
				*truncated = true
				break
			}
			if isSensitiveKey(childKey) {
				out[childKey] = "[redacted]"
				count++
				continue
			}
			out[childKey] = sanitizeDisplayValue(childValue, depth+1, truncated, childKey)
			count++
		}
		return out
	case []any:
		limit := len(value)
		if limit > maxDisplayItems {
			limit = maxDisplayItems
			*truncated = true
		}
		out := make([]any, 0, limit)
		for i := 0; i < limit; i++ {
			out = append(out, sanitizeDisplayValue(value[i], depth+1, truncated, key))
		}
		return out
	case string:
		if isSensitiveKey(key) {
			return "[redacted]"
		}
		runes := []rune(value)
		if len(runes) <= maxDisplayRuneSize {
			return value
		}
		*truncated = true
		return string(runes[:maxDisplayRuneSize]) + "..."
	default:
		return input
	}
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "token") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "password") ||
		strings.Contains(key, "api_key") ||
		strings.Contains(key, "apikey") ||
		strings.Contains(key, "authorization")
}

func toolResultString(output map[string]any, apiErr *model.ErrorBlock) string {
	if apiErr != nil {
		return apiErr.Code + ": " + apiErr.Message
	}
	if len(output) == 0 {
		return "{}"
	}
	b, err := json.Marshal(output)
	if err != nil {
		return "{}"
	}
	return string(b)
}
