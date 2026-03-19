package lanes

import (
	"context"
	"testing"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

func TestSchedulerSerializesSameLane(t *testing.T) {
	scheduler := NewScheduler()
	work := runtimemodel.WorkItem{
		Kind:       runtimemodel.WorkKindEvent,
		EventID:    "evt_1",
		SessionKey: "local:dm:u1",
	}

	first, err := scheduler.Acquire(context.Background(), work)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer first.Release()

	acquired := make(chan Lease, 1)
	errs := make(chan error, 1)
	go func() {
		lease, err := scheduler.Acquire(context.Background(), runtimemodel.WorkItem{
			Kind:       runtimemodel.WorkKindEvent,
			EventID:    "evt_2",
			SessionKey: "local:dm:u1",
		})
		if err != nil {
			errs <- err
			return
		}
		acquired <- lease
	}()

	select {
	case <-acquired:
		t.Fatal("expected second acquire to wait for same lane")
	case err := <-errs:
		t.Fatalf("unexpected acquire error: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	first.Release()

	select {
	case lease := <-acquired:
		defer lease.Release()
		if lease.Key() != Key("session:local:dm:u1") {
			t.Fatalf("unexpected lease key %q", lease.Key())
		}
	case err := <-errs:
		t.Fatalf("unexpected acquire error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for second acquire")
	}
}

func TestSchedulerAllowsDifferentLanes(t *testing.T) {
	scheduler := NewScheduler()
	first, err := scheduler.Acquire(context.Background(), runtimemodel.WorkItem{
		Kind:       runtimemodel.WorkKindEvent,
		EventID:    "evt_1",
		SessionKey: "local:dm:u1",
	})
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer first.Release()

	acquired := make(chan Lease, 1)
	errs := make(chan error, 1)
	go func() {
		lease, err := scheduler.Acquire(context.Background(), runtimemodel.WorkItem{
			Kind:       runtimemodel.WorkKindEvent,
			EventID:    "evt_2",
			SessionKey: "local:dm:u2",
		})
		if err != nil {
			errs <- err
			return
		}
		acquired <- lease
	}()

	select {
	case lease := <-acquired:
		defer lease.Release()
		if lease.Key() != Key("session:local:dm:u2") {
			t.Fatalf("unexpected lease key %q", lease.Key())
		}
	case err := <-errs:
		t.Fatalf("unexpected acquire error: %v", err)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected different lane acquire to proceed immediately")
	}
}
