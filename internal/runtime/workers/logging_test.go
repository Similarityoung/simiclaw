package workers

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

type recoveryRepoStub struct {
	ids []string
	err error
}

func (r recoveryRepoStub) BeatHeartbeat(context.Context, string, time.Time) error {
	return nil
}

func (r recoveryRepoStub) RecoverExpiredProcessing(context.Context, time.Time, time.Time) ([]string, error) {
	return r.ids, r.err
}

type recoveryQueueStub struct {
	reject map[string]bool
}

func (q recoveryQueueStub) TryEnqueue(eventID string) bool {
	return !q.reject[eventID]
}

func TestProcessingRecoveryWorkerLogsRecoveredSummary(t *testing.T) {
	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		worker := NewProcessingRecoveryWorker(
			recoveryRepoStub{ids: []string{"evt_1", "evt_2"}},
			recoveryQueueStub{reject: map[string]bool{"evt_2": true}},
		)
		worker.tick(context.Background(), time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC))
		_ = logging.Sync()
	})

	if !strings.Contains(out, "[runtime.worker] processing recovered") {
		t.Fatalf("expected processing recovered log, got %q", out)
	}
	for _, part := range []string{
		`"worker": "processing_recovery"`,
		`"count": 2`,
		`"deferred": 1`,
		`"enqueued": 1`,
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("missing %q in %q", part, out)
		}
	}
}

type scheduledRepoStub struct {
	job       runtimemodel.ClaimedJob
	ok        bool
	err       error
	failCalls int
}

func (r *scheduledRepoStub) BeatHeartbeat(context.Context, string, time.Time) error {
	return nil
}

func (r *scheduledRepoStub) ClaimRuntimeScheduledJob(context.Context, model.ScheduledJobKind, string, time.Time) (runtimemodel.ClaimedJob, bool, error) {
	return r.job, r.ok, r.err
}

func (r *scheduledRepoStub) FailScheduledJob(context.Context, string, string, time.Time, time.Time) error {
	r.failCalls++
	return nil
}

func (r *scheduledRepoStub) CompleteRuntimeScheduledJob(context.Context, runtimemodel.ClaimedJob, time.Time) error {
	return nil
}

func (r *scheduledRepoStub) MarkEventQueued(context.Context, string, time.Time) error {
	return nil
}

type scheduledIngestStub struct {
	result IngestResult
	err    error
}

func (s scheduledIngestStub) Ingest(context.Context, IngestRequest) (IngestResult, error) {
	return s.result, s.err
}

func TestRunScheduledKindLogsJobOutcome(t *testing.T) {
	repo := &scheduledRepoStub{
		ok: true,
		job: runtimemodel.ClaimedJob{
			JobID: "job_1",
			Kind:  model.ScheduledJobKindCron,
			Payload: runtimemodel.ScheduledJobPayload{
				Source:       "cron",
				Conversation: model.Conversation{ConversationID: "cron-conv", ChannelType: "dm", ParticipantID: "u1"},
				Payload:      model.EventPayload{Type: "cron_fire", Text: "nightly"},
			},
		},
	}

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		RunScheduledKind(context.Background(), repo, scheduledIngestStub{
			result: IngestResult{EventID: "evt_job", Duplicate: false, Enqueued: true},
		}, nil, model.ScheduledJobKindCron, time.Date(2026, 3, 20, 10, 5, 0, 0, time.UTC))
		_ = logging.Sync()
	})

	logcapture.AssertContainsInOrder(t, out,
		"[runtime.worker] job claimed",
		"[runtime.worker] job enqueued",
	)
	for _, part := range []string{
		`"payload_type": "cron_fire"`,
		`"job_id": "job_1"`,
		`"event_id": "evt_job"`,
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("missing %q in %q", part, out)
		}
	}
}

func TestRunScheduledKindLogsIngestFailure(t *testing.T) {
	repo := &scheduledRepoStub{
		ok: true,
		job: runtimemodel.ClaimedJob{
			JobID: "job_fail",
			Kind:  model.ScheduledJobKindRetry,
			Payload: runtimemodel.ScheduledJobPayload{
				Source:       "cron",
				Conversation: model.Conversation{ConversationID: "retry-conv", ChannelType: "dm", ParticipantID: "u1"},
				Payload:      model.EventPayload{Type: "message", Text: "retry"},
			},
		},
	}

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		RunScheduledKind(context.Background(), repo, scheduledIngestStub{
			err: errors.New("gateway unavailable"),
		}, nil, model.ScheduledJobKindRetry, time.Date(2026, 3, 20, 10, 6, 0, 0, time.UTC))
		_ = logging.Sync()
	})

	if repo.failCalls != 1 {
		t.Fatalf("expected scheduled job failure to be recorded, got %d", repo.failCalls)
	}
	if !strings.Contains(out, "[runtime.worker] job ingest failed") {
		t.Fatalf("expected ingest failure log, got %q", out)
	}
	if !strings.Contains(out, `"job_id": "job_fail"`) {
		t.Fatalf("expected job_id in log, got %q", out)
	}
}
