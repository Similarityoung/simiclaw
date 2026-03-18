package kernel

import (
	"context"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

// Facts defines the write-side runtime facts surface consumed by the kernel.
//
// Implementations may live in internal/store, but callers depend only on this
// consumer-owned contract.
type Facts interface {
	ListRunnable(ctx context.Context, limit int) ([]runtimemodel.WorkItem, error)
	ClaimWork(ctx context.Context, work runtimemodel.WorkItem, runID string, now time.Time) (runtimemodel.ClaimContext, bool, error)
	Finalize(ctx context.Context, cmd runtimemodel.FinalizeCommand) error
	GetEventRecord(ctx context.Context, eventID string) (runtimemodel.EventRecord, bool, error)
}
