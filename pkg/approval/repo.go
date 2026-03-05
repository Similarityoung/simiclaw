package approval

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/store"
)

type Repo struct {
	mu         sync.Mutex
	pendingDir string
	doneDir    string
}

func NewRepo(workspace string) *Repo {
	return &Repo{
		pendingDir: filepath.Join(workspace, "runtime", "approvals", "pending"),
		doneDir:    filepath.Join(workspace, "runtime", "approvals", "done"),
	}
}

func (r *Repo) Create(rec model.ApprovalRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec.Status == "" {
		rec.Status = model.ApprovalStatusPending
	}
	return r.saveLocked(rec)
}

func (r *Repo) Save(rec model.ApprovalRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveLocked(rec)
}

func (r *Repo) Get(approvalID string) (model.ApprovalRecord, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getLocked(approvalID)
}

func (r *Repo) List() ([]model.ApprovalRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	pending, err := r.readDirLocked(r.pendingDir)
	if err != nil {
		return nil, err
	}
	done, err := r.readDirLocked(r.doneDir)
	if err != nil {
		return nil, err
	}
	out := make([]model.ApprovalRecord, 0, len(pending)+len(done))
	out = append(out, pending...)
	out = append(out, done...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ApprovalID > out[j].ApprovalID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (r *Repo) getLocked(approvalID string) (model.ApprovalRecord, bool, error) {
	for _, path := range []string{
		filepath.Join(r.pendingDir, approvalID+".json"),
		filepath.Join(r.doneDir, approvalID+".json"),
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return model.ApprovalRecord{}, false, err
		}
		var rec model.ApprovalRecord
		if err := decodeJSON(b, &rec); err != nil {
			return model.ApprovalRecord{}, false, err
		}
		return rec, true, nil
	}
	return model.ApprovalRecord{}, false, nil
}

func (r *Repo) saveLocked(rec model.ApprovalRecord) error {
	if rec.Status == model.ApprovalStatusPending {
		if err := store.AtomicWriteJSON(filepath.Join(r.pendingDir, rec.ApprovalID+".json"), rec, 0o644); err != nil {
			return err
		}
		_ = os.Remove(filepath.Join(r.doneDir, rec.ApprovalID+".json"))
		return nil
	}
	if err := store.AtomicWriteJSON(filepath.Join(r.doneDir, rec.ApprovalID+".json"), rec, 0o644); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(r.pendingDir, rec.ApprovalID+".json"))
	return nil
}

func (r *Repo) readDirLocked(dir string) ([]model.ApprovalRecord, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]model.ApprovalRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var rec model.ApprovalRecord
		if err := decodeJSON(b, &rec); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}
