package delivery

import "testing"

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
