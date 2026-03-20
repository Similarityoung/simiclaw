package delivery

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/internal/testutil/logcapture"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestWorkerExposesRoleMetadata(t *testing.T) {
	worker := NewWorker(nil, nil)
	role := worker.Role()
	if role.Name != "delivery_poll" {
		t.Fatalf("unexpected role name: %+v", role)
	}
	if role.HeartbeatName != "outbox_retry" {
		t.Fatalf("unexpected heartbeat name: %+v", role)
	}
	if role.PollCadence != pollTick {
		t.Fatalf("unexpected poll cadence: %+v", role)
	}
	if role.FailurePolicy != "retry with bounded exponential backoff and dead-letter after max attempts" {
		t.Fatalf("unexpected failure policy: %+v", role)
	}
}

type workerRepoStub struct {
	claimed       runtimemodel.ClaimedOutbox
	claimOK       bool
	claimErr      error
	completed     int
	failed        int
	lastDead      bool
	lastNextRetry time.Time
}

func (r *workerRepoStub) BeatHeartbeat(context.Context, string, time.Time) error {
	return nil
}

func (r *workerRepoStub) RecoverExpiredSending(context.Context, time.Time, time.Time) error {
	return nil
}

func (r *workerRepoStub) ClaimRuntimeOutbox(context.Context, string, time.Time) (runtimemodel.ClaimedOutbox, bool, error) {
	return r.claimed, r.claimOK, r.claimErr
}

func (r *workerRepoStub) FailOutboxSend(_ context.Context, _, _ string, _ string, dead bool, nextAttemptAt, _ time.Time) error {
	r.failed++
	r.lastDead = dead
	r.lastNextRetry = nextAttemptAt
	return nil
}

func (r *workerRepoStub) CompleteOutboxSend(context.Context, string, string, time.Time) error {
	r.completed++
	return nil
}

type senderStub struct {
	err error
}

func (s senderStub) Send(context.Context, model.OutboxMessage) error {
	return s.err
}

func TestWorkerLogsSendSuccess(t *testing.T) {
	repo := &workerRepoStub{
		claimOK: true,
		claimed: runtimemodel.ClaimedOutbox{
			OutboxID:     "out_1",
			EventID:      "evt_1",
			SessionKey:   "session:1",
			Channel:      "telegram",
			TargetID:     "42",
			AttemptCount: 1,
		},
	}

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		worker := NewWorker(repo, senderStub{})
		worker.now = func() time.Time { return time.Date(2026, 3, 20, 8, 0, 0, 0, time.UTC) }
		worker.tick(context.Background())
		_ = logging.Sync()
	})

	if repo.completed != 1 {
		t.Fatalf("expected completed send, got %d", repo.completed)
	}
	if !strings.Contains(out, "[outbound.delivery] send started") || !strings.Contains(out, "[outbound.delivery] sent") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestWorkerLogsRetryScheduled(t *testing.T) {
	repo := &workerRepoStub{
		claimOK: true,
		claimed: runtimemodel.ClaimedOutbox{
			OutboxID:     "out_2",
			EventID:      "evt_2",
			SessionKey:   "session:2",
			Channel:      "telegram",
			TargetID:     "84",
			AttemptCount: 1,
		},
	}

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		worker := NewWorker(repo, senderStub{err: errors.New("telegram timeout")})
		worker.now = func() time.Time { return time.Date(2026, 3, 20, 8, 1, 0, 0, time.UTC) }
		worker.tick(context.Background())
		_ = logging.Sync()
	})

	if repo.failed != 1 {
		t.Fatalf("expected failed send record, got %d", repo.failed)
	}
	if !strings.Contains(out, "[outbound.delivery] retry scheduled") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, `"event_id": "evt_2"`) || !strings.Contains(out, `"outbox_id": "out_2"`) {
		t.Fatalf("missing correlation fields in %q", out)
	}
}

func TestWorkerLogsDeadLetter(t *testing.T) {
	repo := &workerRepoStub{
		claimOK: true,
		claimed: runtimemodel.ClaimedOutbox{
			OutboxID:     "out_dead",
			EventID:      "evt_dead",
			SessionKey:   "session:dead",
			Channel:      "telegram",
			TargetID:     "99",
			AttemptCount: 5,
		},
	}

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		worker := NewWorker(repo, senderStub{err: errors.New("telegram rejected")})
		worker.now = func() time.Time { return time.Date(2026, 3, 20, 8, 2, 0, 0, time.UTC) }
		worker.tick(context.Background())
		_ = logging.Sync()
	})

	if repo.failed != 1 || !repo.lastDead {
		t.Fatalf("expected dead-letter failure record, got failed=%d dead=%v", repo.failed, repo.lastDead)
	}
	if !strings.Contains(out, "[outbound.delivery] dead-lettered") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, `"event_id": "evt_dead"`) || !strings.Contains(out, `"dead": true`) {
		t.Fatalf("missing dead-letter summary in %q", out)
	}
}
