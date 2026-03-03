package store

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type CommitRequest struct {
	SessionKey string
	SessionID  string
	Entries    []model.SessionEntry
	RunTrace   model.RunTrace
	Now        time.Time
}

type commitResult struct {
	CommitID string
	Err      error
}

type commitTask struct {
	Req CommitRequest
	Ack chan commitResult
}

type StoreLoop struct {
	workspace   string
	sessions    *SessionStore
	ch          chan commitTask
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	mu          sync.Mutex
	orderRecord []string
}

func NewStoreLoop(workspace string, sessions *SessionStore) *StoreLoop {
	ctx, cancel := context.WithCancel(context.Background())
	return &StoreLoop{
		workspace: workspace,
		sessions:  sessions,
		ch:        make(chan commitTask, 128),
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (s *StoreLoop) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.ctx.Done():
				return
			case task := <-s.ch:
				commitID, err := s.commit(task.Req)
				task.Ack <- commitResult{CommitID: commitID, Err: err}
			}
		}
	}()
}

func (s *StoreLoop) Stop() {
	s.cancel()
	s.wg.Wait()
}

func (s *StoreLoop) Commit(ctx context.Context, req CommitRequest) (string, error) {
	ack := make(chan commitResult, 1)
	task := commitTask{Req: req, Ack: ack}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case s.ch <- task:
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-ack:
		return res.CommitID, res.Err
	}
}

func (s *StoreLoop) commit(req CommitRequest) (string, error) {
	if req.Now.IsZero() {
		req.Now = time.Now().UTC()
	}
	commitID := fmt.Sprintf("c_%d", req.Now.UnixNano())
	lastEntryID := ""
	if len(req.Entries) > 0 {
		lastEntryID = req.Entries[len(req.Entries)-1].EntryID
	}
	commitEntry := model.SessionEntry{
		Type:    "commit",
		EntryID: fmt.Sprintf("e_commit_%d", req.Now.UnixNano()),
		RunID:   req.RunTrace.RunID,
		Commit: &model.CommitMarker{
			CommitID:    commitID,
			RunID:       req.RunTrace.RunID,
			EntryCount:  len(req.Entries),
			LastEntryID: lastEntryID,
		},
	}

	batch := make([]any, 0, len(req.Entries)+1)
	for _, e := range req.Entries {
		batch = append(batch, e)
	}
	batch = append(batch, commitEntry)

	s.recordOrder("append_batch")
	if err := AppendJSONL(s.sessions.SessionFilePath(req.SessionID), batch...); err != nil {
		return "", err
	}

	runPath := filepath.Join(s.workspace, "runtime", "runs", fmt.Sprintf("%s.json", req.RunTrace.RunID))
	s.recordOrder("write_run")
	if err := AtomicWriteJSON(runPath, req.RunTrace, 0o644); err != nil {
		return "", err
	}

	s.recordOrder("update_sessions")
	if err := s.sessions.UpdateIndex(req.SessionKey, req.SessionID, req.Now); err != nil {
		return "", err
	}
	return commitID, nil
}

func (s *StoreLoop) recordOrder(step string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orderRecord = append(s.orderRecord, step)
}

func (s *StoreLoop) OrderRecord() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.orderRecord))
	copy(out, s.orderRecord)
	return out
}
