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

// Processor owns the runtime claim -> execute -> finalize orchestration for one
// already-scheduled work item.
type Processor interface {
	Process(ctx context.Context, work runtimemodel.WorkItem) error
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
