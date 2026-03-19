package workers

import (
	"context"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
)

type stubWorker struct {
	role kernel.WorkerRole
}

func (w stubWorker) Role() kernel.WorkerRole   { return w.role }
func (w stubWorker) Run(context.Context) error { return nil }

func TestRegistryRegisterAndReplaceByRole(t *testing.T) {
	registry := NewRegistry()
	first := stubWorker{role: kernel.WorkerRole{Name: "alpha", PollCadence: time.Second}}
	replacement := stubWorker{role: kernel.WorkerRole{Name: "alpha", PollCadence: 2 * time.Second}}
	second := stubWorker{role: kernel.WorkerRole{Name: "beta", PollCadence: 3 * time.Second}}

	registry.Register(first)
	registry.Register(second)
	registry.Register(replacement)

	workers := registry.All()
	if len(workers) != 2 {
		t.Fatalf("expected 2 workers, got %d", len(workers))
	}
	if workers[0].Role().PollCadence != 2*time.Second {
		t.Fatalf("expected replacement to keep order and update role, got %+v", workers[0].Role())
	}
	if workers[1].Role().Name != "beta" {
		t.Fatalf("expected beta to stay registered, got %+v", workers[1].Role())
	}
}

func TestRegisterBuiltinsAddsNamedWorkers(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltins(registry, Builtins{})

	workers := registry.All()
	if len(workers) != 5 {
		t.Fatalf("expected 5 builtin workers, got %d", len(workers))
	}
	want := []string{
		"heartbeat",
		"processing_recovery",
		"delivery_poll",
		"scheduled_jobs_delayed",
		"scheduled_jobs_cron",
	}
	for idx, name := range want {
		if workers[idx].Role().Name != name {
			t.Fatalf("expected worker[%d]=%q, got %+v", idx, name, workers[idx].Role())
		}
	}
}
