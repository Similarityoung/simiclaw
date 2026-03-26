package provider

import (
	"time"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	"github.com/similarityyoung/simiclaw/pkg/logging"
)

func logTransportDebug(providerName, model, message string, timeout time.Duration, err error) {
	fields := []logging.Field{
		logging.String("provider", providerName),
		logging.String("model", model),
		logging.String("error_kind", string(kernel.CapabilityErrorKindOf(err))),
		logging.Error(err),
	}
	if timeout > 0 {
		fields = append(fields, logging.Int64("timeout_ms", timeout.Milliseconds()))
	}
	logging.L("provider."+providerName).Debug(message, fields...)
}
