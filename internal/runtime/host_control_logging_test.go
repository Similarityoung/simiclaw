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

func TestHostControlLogsWorkerLifecycle(t *testing.T) {
	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		host := newWorkerHost(nil, []kernel.Worker{
			blockingWorker{role: kernel.WorkerRole{Name: "test_worker", HeartbeatName: "test_heartbeat", PollCadence: time.Second}},
		}, logging.L("runtime.host"))
		control := &HostControl{
			host:   host,
			logger: logging.L("runtime.host"),
		}
		if err := control.Start(context.Background()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		control.Stop()
		_ = logging.Sync()
	})

	logcapture.AssertContainsInOrder(t, out,
		"[runtime.host] host starting",
		"[runtime.host] worker starting",
		"[runtime.host] host started",
		"[runtime.host] host stopping",
		"[runtime.host] worker stopped",
		"[runtime.host] host stopped",
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
