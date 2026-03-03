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

type SessionStore struct {
	mu           sync.Mutex
	workspace    string
	sessionsPath string
	seq          atomic.Uint64
	index        model.SessionIndex
}

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
	if err := AppendJSONL(s.sessionFilePath(sessionID), header); err != nil {
		return "", false, err
	}
	s.index.Sessions[sessionKey] = model.SessionIndexRow{ActiveSessionID: sessionID, UpdatedAt: now}
	s.index.UpdatedAt = now
	if err := AtomicWriteJSON(s.sessionsPath, s.index, 0o644); err != nil {
		return "", false, err
	}
	return sessionID, true, nil
}

func (s *SessionStore) UpdateIndex(sessionKey, sessionID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.index.Sessions[sessionKey] = model.SessionIndexRow{ActiveSessionID: sessionID, UpdatedAt: now}
	s.index.UpdatedAt = now
	return AtomicWriteJSON(s.sessionsPath, s.index, 0o644)
}

func (s *SessionStore) SessionFilePath(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionFilePath(sessionID)
}

func (s *SessionStore) sessionFilePath(sessionID string) string {
	return filepath.Join(s.workspace, "runtime", "sessions", fmt.Sprintf("%s.jsonl", sessionID))
}

func (s *SessionStore) newSessionID(now time.Time) string {
	n := s.seq.Add(1)
	return fmt.Sprintf("s_%s_%03d", now.UTC().Format("20060102_150405"), n)
}
