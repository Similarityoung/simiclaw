package runner

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type runIDContextKey struct{}

func WithRunID(ctx context.Context, runID string) context.Context {
	runID = strings.TrimSpace(runID)
	if runID == "" || ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, runIDContextKey{}, runID)
}

func runLogger(ctx context.Context, module string, event model.InternalEvent) *logging.Logger {
	fields := make([]logging.Field, 0, 6)
	if event.EventID != "" {
		fields = append(fields, logging.String("event_id", event.EventID))
	}
	if runID := runIDFromContext(ctx); runID != "" {
		fields = append(fields, logging.String("run_id", runID))
	}
	if event.SessionKey != "" {
		fields = append(fields, logging.String("session_key", event.SessionKey))
	}
	if event.ActiveSessionID != "" {
		fields = append(fields, logging.String("session_id", event.ActiveSessionID))
	}
	if event.Payload.Type != "" {
		fields = append(fields, logging.String("payload_type", event.Payload.Type))
	}
	if event.Conversation.ChannelType != "" {
		fields = append(fields, logging.String("channel", event.Conversation.ChannelType))
	}
	return logging.L(module).With(fields...)
}

func runIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	runID, _ := ctx.Value(runIDContextKey{}).(string)
	return strings.TrimSpace(runID)
}

func providerName(model string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(model), "/")
	return name
}

func providerErrorKind(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "deadline exceeded"), strings.Contains(msg, "timeout"):
			return "timeout"
		case strings.Contains(msg, "context canceled"), strings.Contains(msg, "cancelled"), strings.Contains(msg, "canceled"):
			return "canceled"
		default:
			return "error"
		}
	}
}

func mapSummaryFields(prefix string, input map[string]any, truncated bool) []logging.Field {
	fields := []logging.Field{
		logging.Int(prefix+"_items", len(input)),
		logging.Bool(prefix+"_truncated", truncated),
	}
	if keys := mapSummaryKeys(input); keys != "" {
		fields = append(fields, logging.String(prefix+"_keys", keys))
	}
	return fields
}

func mapSummaryKeys(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}
