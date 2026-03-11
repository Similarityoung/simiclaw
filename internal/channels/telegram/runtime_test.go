package telegram

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/logging"
)

type fakeHeartbeatRecorder struct {
	mu     sync.Mutex
	names  []string
	fail   error
	notify chan struct{}
}

func (r *fakeHeartbeatRecorder) BeatHeartbeat(_ context.Context, name string, _ time.Time) error {
	r.mu.Lock()
	r.names = append(r.names, name)
	notify := r.notify
	fail := r.fail
	r.mu.Unlock()
	if notify != nil {
		select {
		case notify <- struct{}{}:
		default:
		}
	}
	return fail
}

func (r *fakeHeartbeatRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.names)
}

func (r *fakeHeartbeatRecorder) firstName() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.names) == 0 {
		return ""
	}
	return r.names[0]
}

func TestRunHeartbeatBeatsImmediatelyAndPeriodically(t *testing.T) {
	recorder := &fakeHeartbeatRecorder{notify: make(chan struct{}, 8)}
	runtime := &Runtime{
		heartbeat:         recorder,
		heartbeatInterval: 10 * time.Millisecond,
		logger:            logging.L("telegram-test"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	runtime.wg.Add(1)
	go runtime.runHeartbeat(ctx)
	defer func() {
		cancel()
		runtime.wg.Wait()
	}()

	waitForHeartbeatCount(t, recorder, 1)
	if got := recorder.firstName(); got != "telegram_polling" {
		t.Fatalf("expected heartbeat name telegram_polling, got %q", got)
	}
	waitForHeartbeatCount(t, recorder, 2)
}

func TestRunHeartbeatErrorsDoNotStopLoop(t *testing.T) {
	recorder := &fakeHeartbeatRecorder{
		fail:   errors.New("boom"),
		notify: make(chan struct{}, 8),
	}
	runtime := &Runtime{
		heartbeat:         recorder,
		heartbeatInterval: 10 * time.Millisecond,
		logger:            logging.L("telegram-test"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	runtime.wg.Add(1)
	go runtime.runHeartbeat(ctx)
	defer func() {
		cancel()
		runtime.wg.Wait()
	}()

	waitForHeartbeatCount(t, recorder, 2)
}

func waitForHeartbeatCount(t *testing.T, recorder *fakeHeartbeatRecorder, want int) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if recorder.count() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected at least %d heartbeat calls, got %d", want, recorder.count())
}
