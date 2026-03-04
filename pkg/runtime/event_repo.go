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

// EventRepo 维护 event_id 到事件记录的内存索引，并负责落盘到 events.json。
type EventRepo struct {
	mu   sync.RWMutex
	path string
	data map[string]model.EventRecord
}

// eventSnapshot 是 events.json 的文件结构。
type eventSnapshot struct {
	Events map[string]model.EventRecord `json:"events"`
}

// NewEventRepo 初始化事件仓库并加载已有事件数据。
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

// load 从磁盘恢复事件快照到内存。
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

// save 将内存数据原子写回 events.json。
func (r *EventRepo) save() error {
	return store.AtomicWriteJSON(r.path, eventSnapshot{Events: r.data}, 0o644)
}

// Put 写入或覆盖单条事件记录并立即持久化。
func (r *EventRepo) Put(rec model.EventRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[rec.EventID] = rec
	return r.save()
}

// Update 原地修改指定事件记录并刷新 UpdatedAt。
func (r *EventRepo) Update(eventID string, fn func(*model.EventRecord)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.data[eventID]
	if !ok {
		return os.ErrNotExist
	}
	// map value 是值拷贝，修改后需再写回 map。
	fn(&rec)
	rec.UpdatedAt = time.Now().UTC()
	r.data[eventID] = rec
	return r.save()
}

// Get 读取单条事件记录。
func (r *EventRepo) Get(eventID string) (model.EventRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.data[eventID]
	return rec, ok
}

// List 返回按 UpdatedAt 升序排列的事件记录快照。
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
