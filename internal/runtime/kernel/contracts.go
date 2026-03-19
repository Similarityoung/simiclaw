package kernel

import (
	"context"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

// Executor runs already-claimed work outside the storage transaction boundary.
type Executor interface {
	Execute(ctx context.Context, claim runtimemodel.ClaimContext, sink EventSink) (runtimemodel.ExecutionResult, error)
}

// Worker owns one bounded background responsibility inside the runtime.
type Worker interface {
	Role() WorkerRole
	Run(ctx context.Context) error
}

type WorkerRole struct {
	Name          string
	HeartbeatName string
	PollCadence   time.Duration
	FailurePolicy string
}
