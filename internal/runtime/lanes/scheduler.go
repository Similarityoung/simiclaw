package lanes

import (
	"context"
	"sync"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

type Lease struct {
	key     Key
	release func()
	once    sync.Once
}

func (l *Lease) Key() Key {
	if l == nil {
		return ""
	}
	return l.key
}

func (l *Lease) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.release != nil {
			l.release()
		}
	})
}

type Scheduler struct {
	mu    sync.Mutex
	lanes map[Key]*laneState
}

type laneState struct {
	token chan struct{}
	refs  int
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		lanes: make(map[Key]*laneState),
	}
}

// Acquire attempts to acquire a lease for the given work item. It will block until the lease is acquired or the context is canceled.
func (s *Scheduler) Acquire(ctx context.Context, work runtimemodel.WorkItem) (*Lease, error) {
	key := Resolve(work)
	state := s.retain(key)

	select {
	case <-state.token:
		lease := &Lease{key: key}
		lease.release = func() {
			state.token <- struct{}{}
			s.drop(key, state)
		}
		return lease, nil
	case <-ctx.Done():
		s.drop(key, state)
		return nil, ctx.Err()
	}
}

// retain increments the reference count for the given key and returns the lane state. If the lane does not exist, it creates a new one.
func (s *Scheduler) retain(key Key) *laneState {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.lanes[key]
	if !ok {
		state = &laneState{
			token: make(chan struct{}, 1),
		}
		state.token <- struct{}{}
		s.lanes[key] = state
	}
	state.refs++
	return state
}

func (s *Scheduler) drop(key Key, state *laneState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.lanes[key]
	if !ok || current != state {
		return
	}
	state.refs--
	if state.refs == 0 && len(state.token) == 1 {
		delete(s.lanes, key)
	}
}
