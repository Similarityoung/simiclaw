package provider

import (
	"context"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/logging"
)

func logTransportDebug(providerName, model, message string, timeout time.Duration, err error) {
	fields := []logging.Field{
		logging.String("provider", providerName),
		logging.String("model", strings.TrimSpace(model)),
		logging.String("error_kind", transportErrorKind(err)),
		logging.Error(err),
	}
	if timeout > 0 {
		fields = append(fields, logging.Int64("timeout_ms", timeout.Milliseconds()))
	}
	logging.L("provider."+providerName).Debug(message, fields...)
}

func transportErrorKind(err error) string {
	switch {
	case err == nil:
		return ""
	case err == context.DeadlineExceeded:
		return "timeout"
	case err == context.Canceled:
		return "canceled"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "deadline exceeded"), strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "context canceled"), strings.Contains(msg, "context cancelled"), strings.Contains(msg, "canceled"), strings.Contains(msg, "cancelled"):
		return "canceled"
	default:
		return "error"
	}
}
