package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/store"
)

type EventRepo struct {
	mu   sync.RWMutex
	path string
	data map[string]model.EventRecord
}

type eventSnapshot struct {
	Events map[string]model.EventRecord `json:"events"`
}

func NewEventRepo(workspace string) (*EventRepo, error) {
	r := &EventRepo{
		path: filepath.Join(workspace, "runtime", "events", "events.json"),
		data: map[string]model.EventRecord{},
	}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *EventRepo) load() error {
	b, err := os.ReadFile(r.path)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	var snap eventSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return err
	}
	if snap.Events != nil {
		r.data = snap.Events
	}
	return nil
}

func (r *EventRepo) save() error {
	return store.AtomicWriteJSON(r.path, eventSnapshot{Events: r.data}, 0o644)
}

func (r *EventRepo) Put(rec model.EventRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[rec.EventID] = rec
	return r.save()
}

func (r *EventRepo) Update(eventID string, fn func(*model.EventRecord)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.data[eventID]
	if !ok {
		return os.ErrNotExist
	}
	fn(&rec)
	rec.UpdatedAt = time.Now().UTC()
	r.data[eventID] = rec
	return r.save()
}

func (r *EventRepo) Get(eventID string) (model.EventRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.data[eventID]
	return rec, ok
}

func (r *EventRepo) List() []model.EventRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.EventRecord, 0, len(r.data))
	for _, v := range r.data {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.Before(out[j].UpdatedAt)
	})
	return out
}
