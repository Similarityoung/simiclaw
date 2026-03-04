package gateway

import (
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

var (
	tgKeyRE  = regexp.MustCompile(`^telegram:update:[0-9]+$`)
	cliKeyRE = regexp.MustCompile(`^cli:[^:]+:[0-9]+$`)
)

const maxTimestampDriftWindow = 10 * time.Minute

func validateRequest(req model.IngestRequest, now time.Time) (time.Time, *APIError) {
	if req.Source == "" {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "field source is required", Details: map[string]any{"field": "source"}}
	}
	if req.Conversation.ConversationID == "" {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "field conversation.conversation_id is required", Details: map[string]any{"field": "conversation.conversation_id"}}
	}
	if req.Conversation.ChannelType == "" {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "field conversation.channel_type is required", Details: map[string]any{"field": "conversation.channel_type"}}
	}
	if req.Conversation.ChannelType == "channel" {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "channel_type 'channel' is reserved", Details: map[string]any{"field": "conversation.channel_type"}}
	}
	if req.Conversation.ChannelType == "dm" && req.Conversation.ParticipantID == "" {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "field conversation.participant_id is required", Details: map[string]any{"field": "conversation.participant_id"}}
	}
	if req.Payload.Type == "" {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "field payload.type is required", Details: map[string]any{"field": "payload.type"}}
	}
	if req.IdempotencyKey == "" {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "field idempotency_key is required", Details: map[string]any{"field": "idempotency_key"}}
	}
	if req.Source == "telegram" && !tgKeyRE.MatchString(req.IdempotencyKey) {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "invalid telegram idempotency_key format"}
	}
	if req.Source == "cli" && !cliKeyRE.MatchString(req.IdempotencyKey) {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "invalid cli idempotency_key format"}
	}
	if req.Payload.NativeRef != "" {
		clean := filepath.Clean(req.Payload.NativeRef)
		if strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) || !strings.HasPrefix(clean, "runtime/native/") {
			return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "native_ref must stay within runtime/native/**"}
		}
	}
	if req.Timestamp == "" {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "field timestamp is required", Details: map[string]any{"field": "timestamp"}}
	}
	if !strings.HasSuffix(req.Timestamp, "Z") {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "timestamp must be UTC"}
	}
	ts, err := time.Parse(time.RFC3339, req.Timestamp)
	if err != nil {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "invalid timestamp"}
	}
	if diff := now.Sub(ts); diff > maxTimestampDriftWindow || diff < -maxTimestampDriftWindow {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "timestamp drift exceeds 10m"}
	}
	return ts, nil
}
