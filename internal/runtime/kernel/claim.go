package kernel

import (
	"context"
	"fmt"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

func (s *Service) claim(ctx context.Context, work runtimemodel.WorkItem) (runtimemodel.ClaimContext, time.Time, bool, error) {
	now := s.now()
	runID := fmt.Sprintf("run_%d_%d", now.UnixNano(), s.next())
	claim, ok, err := s.facts.ClaimWork(ctx, work, runID, now)
	if claim.RunID == "" {
		claim.RunID = runID
	}
	return claim, now, ok, err
}
