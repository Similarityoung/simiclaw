package tx

import (
	"testing"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/internal/query"
	"github.com/similarityyoung/simiclaw/internal/runtime"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	storequeries "github.com/similarityyoung/simiclaw/internal/store/queries"
)

func TestRuntimeRepositoryImplementsKernelAndWorkerContracts(t *testing.T) {
	var (
		_ kernel.Facts                = (*RuntimeRepository)(nil)
		_ gateway.Repository          = (*RuntimeRepository)(nil)
		_ runtime.WorkerRepository    = (*RuntimeRepository)(nil)
		_ runtime.ReadinessRepository = (*RuntimeRepository)(nil)
	)
}

func TestStoreQueriesRepositoryImplementsSurfaceQueryContracts(t *testing.T) {
	var (
		_ query.EventRepository   = (*storequeries.Repository)(nil)
		_ query.RunRepository     = (*storequeries.Repository)(nil)
		_ query.SessionRepository = (*storequeries.Repository)(nil)
	)
}
