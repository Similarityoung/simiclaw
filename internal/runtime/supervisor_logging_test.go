package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	"github.com/similarityyoung/simiclaw/internal/testutil/logcapture"
	"github.com/similarityyoung/simiclaw/pkg/logging"
)

type blockingWorker struct {
	role kernel.WorkerRole
}

func (w blockingWorker) Role() kernel.WorkerRole {
	return w.role
}

func (w blockingWorker) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func TestSupervisorLogsWorkerLifecycle(t *testing.T) {
	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		supervisor := &Supervisor{
			background: []kernel.Worker{
				blockingWorker{role: kernel.WorkerRole{Name: "test_worker", HeartbeatName: "test_heartbeat", PollCadence: time.Second}},
			},
			logger: logging.L("runtime.supervisor"),
		}
		if err := supervisor.Start(context.Background()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		supervisor.Stop()
		_ = logging.Sync()
	})

	logcapture.AssertContainsInOrder(t, out,
		"[runtime.supervisor] supervisor starting",
		"[runtime.supervisor] worker starting",
		"[runtime.supervisor] supervisor started",
		"[runtime.supervisor] supervisor stopping",
		"[runtime.supervisor] worker stopped",
		"[runtime.supervisor] supervisor stopped",
	)
	for _, part := range []string{
		`"worker": "test_worker"`,
		`"heartbeat": "test_heartbeat"`,
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("expected %q in %q", part, out)
		}
	}
}
