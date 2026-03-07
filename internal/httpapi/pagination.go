package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const (
	defaultPageLimit = 50
	maxPageLimit     = 200
)

func parsePageLimit(raw string) (int, *gateway.APIError) {
	if raw == "" {
		return defaultPageLimit, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 || v > maxPageLimit {
		return 0, &gateway.APIError{
			StatusCode: http.StatusBadRequest,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    fmt.Sprintf("invalid limit: %q", raw),
			Details:    map[string]any{"field": "limit"},
		}
	}
	return v, nil
}

func decodeCursor(raw string, dst any) *gateway.APIError {
	if raw == "" {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return &gateway.APIError{
			StatusCode: http.StatusBadRequest,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    "invalid cursor",
			Details:    map[string]any{"field": "cursor"},
		}
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return &gateway.APIError{
			StatusCode: http.StatusBadRequest,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    "invalid cursor",
			Details:    map[string]any{"field": "cursor"},
		}
	}
	return nil
}

func encodeCursor(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}
