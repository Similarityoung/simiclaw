package tx

import (
	"testing"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/internal/gateway/bindings"
	"github.com/similarityyoung/simiclaw/internal/runtime"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
)

func TestRuntimeRepositoryImplementsKernelAndWorkerContracts(t *testing.T) {
	var (
		_ kernel.Facts                = (*RuntimeRepository)(nil)
		_ gateway.Repository          = (*RuntimeRepository)(nil)
		_ bindings.SessionLookup      = (*RuntimeRepository)(nil)
		_ runtime.WorkerRepository    = (*RuntimeRepository)(nil)
		_ runtime.ReadinessRepository = (*RuntimeRepository)(nil)
	)
}
