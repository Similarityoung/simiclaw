package workers

import "github.com/similarityyoung/simiclaw/internal/runtime/kernel"

type Registry struct {
	workers []kernel.Worker
	index   map[string]int
}

type Builtins struct {
	Heartbeat  HeartbeatRepository
	Processing ProcessingRecoveryRepository
	Delivery   DeliveryPollRepository
	Scheduled  ScheduledJobRepository
	Ingest     EventIngestor
	Queue      EventEnqueuer
	Sender     Sender
}

func NewRegistry() *Registry {
	return &Registry{index: make(map[string]int)}
}

func (r *Registry) Register(worker kernel.Worker) {
	if r == nil || worker == nil {
		return
	}
	role := worker.Role().Name
	if idx, ok := r.index[role]; ok {
		r.workers[idx] = worker
		return
	}
	r.index[role] = len(r.workers)
	r.workers = append(r.workers, worker)
}

func (r *Registry) All() []kernel.Worker {
	if r == nil || len(r.workers) == 0 {
		return nil
	}
	out := make([]kernel.Worker, len(r.workers))
	copy(out, r.workers)
	return out
}

func RegisterBuiltins(r *Registry, builtins Builtins) {
	if r == nil {
		return
	}
	r.Register(NewHeartbeatWorker(builtins.Heartbeat))
	r.Register(NewProcessingRecoveryWorker(builtins.Processing, builtins.Queue))
	r.Register(NewDeliveryPollWorker(builtins.Delivery, builtins.Sender))
	r.Register(NewDelayedJobsWorker(builtins.Scheduled, builtins.Ingest, builtins.Queue))
	r.Register(NewCronWorker(builtins.Scheduled, builtins.Ingest, builtins.Queue))
}
