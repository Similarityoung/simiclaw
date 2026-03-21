package kernel

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestServiceProcessConcurrentCallsKeepIDsUnique(t *testing.T) {
	const workers = 24

	facts := &concurrentFacts{}
	executor := &stubExecutor{
		result: runtimemodel.ExecutionResult{
			RunMode: model.RunModeNormal,
			OutputMessages: []runtimemodel.StoredMessage{
				{Role: "assistant", Content: "ok", Visible: true},
			},
		},
	}
	svc := NewService(facts, executor, NopEventSink{})
	svc.SetClock(func() time.Time {
		return time.Date(2026, 3, 18, 11, 0, 0, 0, time.UTC)
	})

	var wg sync.WaitGroup
	for i := range workers {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			work := runtimemodel.WorkItem{
				EventID: fmt.Sprintf("evt_%02d", i),
			}
			if err := svc.Process(context.Background(), work); err != nil {
				t.Errorf("Process(%s): %v", work.EventID, err)
			}
		}()
	}
	wg.Wait()

	facts.mu.Lock()
	defer facts.mu.Unlock()
	if len(facts.finalizeCmds) != workers {
		t.Fatalf("expected %d finalize commands, got %d", workers, len(facts.finalizeCmds))
	}
	runIDs := make(map[string]struct{}, workers)
	messageIDs := make(map[string]struct{}, workers)
	for _, finalize := range facts.finalizeCmds {
		if _, exists := runIDs[finalize.RunID]; exists {
			t.Fatalf("duplicate run id detected: %q", finalize.RunID)
		}
		runIDs[finalize.RunID] = struct{}{}
		if len(finalize.Messages) != 1 {
			t.Fatalf("expected exactly one stored message per run, got %+v", finalize)
		}
		messageID := finalize.Messages[0].MessageID
		if _, exists := messageIDs[messageID]; exists {
			t.Fatalf("duplicate message id detected: %q", messageID)
		}
		messageIDs[messageID] = struct{}{}
	}
}

type concurrentFacts struct {
	mu           sync.Mutex
	finalizeCmds []runtimemodel.FinalizeCommand
}

func (f *concurrentFacts) ListRunnable(context.Context, int) ([]runtimemodel.WorkItem, error) {
	return nil, nil
}

func (f *concurrentFacts) ClaimWork(_ context.Context, work runtimemodel.WorkItem, runID string, _ time.Time) (runtimemodel.ClaimContext, bool, error) {
	return runtimemodel.ClaimContext{
		Work: work,
		Event: model.InternalEvent{
			EventID:         work.EventID,
			SessionKey:      "session:" + work.EventID,
			ActiveSessionID: "ses:" + work.EventID,
		},
		RunID:      runID,
		RunMode:    model.RunModeNormal,
		SessionKey: "session:" + work.EventID,
		SessionID:  "ses:" + work.EventID,
	}, true, nil
}

func (f *concurrentFacts) Finalize(_ context.Context, cmd runtimemodel.FinalizeCommand) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finalizeCmds = append(f.finalizeCmds, cmd)
	return nil
}

func (f *concurrentFacts) GetEventRecord(context.Context, string) (runtimemodel.EventRecord, bool, error) {
	return runtimemodel.EventRecord{}, false, nil
}
