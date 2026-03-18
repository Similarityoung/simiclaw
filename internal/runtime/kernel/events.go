package kernel

import (
	"context"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

// EventSink receives runtime lifecycle events emitted by the kernel.
//
// Streaming, SSE, logs, and other observers should hang off this boundary
// instead of being embedded in the execution loop itself.
type EventSink interface {
	Publish(ctx context.Context, event runtimemodel.RuntimeEvent) error
}

type NopEventSink struct{}

func (NopEventSink) Publish(context.Context, runtimemodel.RuntimeEvent) error {
	return nil
}
