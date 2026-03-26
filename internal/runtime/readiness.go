package runtime

import (
	"context"
	"fmt"
	"time"
)

const heartbeatFreshness = 30 * time.Second

type ReadinessRepository interface {
	CheckReadWrite(ctx context.Context) error
	HeartbeatAt(ctx context.Context, workerName string) (time.Time, bool, error)
}

type readinessHost interface {
	Alive() bool
	InboundDepth() int
	WorkerHeartbeatNames() []string
}

type ReadinessProbe struct {
	repo                ReadinessRepository
	host                readinessHost
	extraHeartbeatNames []string
}

func NewReadinessProbe(repo ReadinessRepository, host readinessHost, extraHeartbeatNames ...string) *ReadinessProbe {
	return &ReadinessProbe{
		repo:                repo,
		host:                host,
		extraHeartbeatNames: extraHeartbeatNames,
	}
}

func (p *ReadinessProbe) ReadyState(ctx context.Context) (map[string]any, error) {
	queueDepth := 0
	if p != nil && p.host != nil {
		queueDepth = p.host.InboundDepth()
	}
	state := map[string]any{
		"status":      "ready",
		"queue_depth": queueDepth,
		"time":        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if p == nil || p.repo == nil {
		state["status"] = "not_ready"
		state["db_error"] = "readiness repository unavailable"
		return state, fmt.Errorf("readiness repository unavailable")
	}
	if err := p.repo.CheckReadWrite(ctx); err != nil {
		state["status"] = "not_ready"
		state["db_error"] = err.Error()
		return state, err
	}
	if p.host == nil || !p.host.Alive() {
		state["status"] = "not_ready"
		state["event_loop"] = "down"
		return state, fmt.Errorf("event loop down")
	}

	workers := append([]string(nil), p.host.WorkerHeartbeatNames()...)
	workers = append(workers, p.extraHeartbeatNames...)
	for _, worker := range workers {
		beatAt, ok, err := p.repo.HeartbeatAt(ctx, worker)
		if err != nil {
			state[worker] = "error"
			continue
		}
		if !ok {
			state[worker] = "missing"
			continue
		}
		if time.Since(beatAt) > heartbeatFreshness {
			state[worker] = "stale"
		} else {
			state[worker] = "alive"
		}
	}
	return state, nil
}
