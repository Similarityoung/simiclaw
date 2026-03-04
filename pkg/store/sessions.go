package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

// SessionStore 维护 session_key 到活跃 session_id 的索引，并负责会话文件路径管理。
type SessionStore struct {
	mu           sync.Mutex
	workspace    string
	sessionsPath string
	seq          atomic.Uint64
	index        model.SessionIndex
}

// NewSessionStore 创建会话存储并从 sessions.json 加载内存索引。
func NewSessionStore(workspace string) (*SessionStore, error) {
	s := &SessionStore{
		workspace:    workspace,
		sessionsPath: filepath.Join(workspace, "runtime", "sessions.json"),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load 读取会话索引文件；空文件时会初始化为默认结构。
func (s *SessionStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var idx model.SessionIndex
	b, err := os.ReadFile(s.sessionsPath)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		idx = model.SessionIndex{FormatVersion: "1", UpdatedAt: time.Now().UTC(), Sessions: map[string]model.SessionIndexRow{}}
	} else {
		if err := decodeJSON(b, &idx); err != nil {
			return err
		}
		if idx.Sessions == nil {
			idx.Sessions = map[string]model.SessionIndexRow{}
		}
	}
	s.index = idx
	return nil
}

// ResolveSession 返回已有 session，或为首次出现的 session_key 创建新 session。
func (s *SessionStore) ResolveSession(sessionKey string, now time.Time) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if row, ok := s.index.Sessions[sessionKey]; ok {
		return row.ActiveSessionID, false, nil
	}
	sessionID := s.newSessionID(now)
	header := model.SessionHeader{
		Type:          "header",
		SessionID:     sessionID,
		SessionKey:    sessionKey,
		CreatedAt:     now,
		FormatVersion: "1",
	}
	// 先写入会话头记录，确保该 session 文件可追踪来源与版本。
	if err := AppendJSONL(s.sessionFilePath(sessionID), header); err != nil {
		return "", false, err
	}
	// 再更新索引，建立 session_key -> active_session_id 的映射关系。
	s.index.Sessions[sessionKey] = model.SessionIndexRow{ActiveSessionID: sessionID, UpdatedAt: now}
	s.index.UpdatedAt = now
	if err := AtomicWriteJSON(s.sessionsPath, s.index, 0o644); err != nil {
		return "", false, err
	}
	return sessionID, true, nil
}

// UpdateIndex 刷新 session_key 的活跃会话指针并持久化索引。
func (s *SessionStore) UpdateIndex(sessionKey, sessionID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.index.Sessions[sessionKey] = model.SessionIndexRow{ActiveSessionID: sessionID, UpdatedAt: now}
	s.index.UpdatedAt = now
	return AtomicWriteJSON(s.sessionsPath, s.index, 0o644)
}

// SessionFilePath 返回指定 session_id 对应的 JSONL 文件路径。
func (s *SessionStore) SessionFilePath(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionFilePath(sessionID)
}

// sessionFilePath 在不加锁的前提下拼接会话文件路径（调用方需自行保证并发安全）。
func (s *SessionStore) sessionFilePath(sessionID string) string {
	return filepath.Join(s.workspace, "runtime", "sessions", fmt.Sprintf("%s.jsonl", sessionID))
}

// newSessionID 生成单进程内递增且带时间戳的 session_id。
func (s *SessionStore) newSessionID(now time.Time) string {
	n := s.seq.Add(1)
	return fmt.Sprintf("s_%s_%03d", now.UTC().Format("20060102_150405"), n)
}
