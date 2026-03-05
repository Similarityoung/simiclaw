package persistence

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

const defaultDMScope = "default"

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

	now := time.Now().UTC()
	var idx model.SessionIndex
	b, err := os.ReadFile(s.sessionsPath)
	if err != nil || len(b) == 0 {
		recovered, recoverErr := RecoverSessionIndex(s.workspace, model.SessionIndex{}, now)
		if recoverErr != nil {
			if err != nil {
				return err
			}
			return recoverErr
		}
		s.index = recovered
		return nil
	}

	if err := decodeJSON(b, &idx); err != nil {
		recovered, recoverErr := RecoverSessionIndex(s.workspace, model.SessionIndex{}, now)
		if recoverErr != nil {
			return err
		}
		s.index = recovered
		return nil
	}
	recovered, err := RecoverSessionIndex(s.workspace, idx, now)
	if err != nil {
		return err
	}
	s.index = recovered
	return nil
}

// ResolveSession 返回已有 session，或为首次出现的 session_key 创建新 session。
func (s *SessionStore) ResolveSession(sessionKey string, conv model.Conversation, dmScope string, now time.Time) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if dmScope == "" {
		dmScope = defaultDMScope
	}
	if row, ok := s.index.Sessions[sessionKey]; ok {
		changed := false
		if row.ConversationID == "" && conv.ConversationID != "" {
			row.ConversationID = conv.ConversationID
			changed = true
		}
		if row.ChannelType == "" && conv.ChannelType != "" {
			row.ChannelType = conv.ChannelType
			changed = true
		}
		if row.ParticipantID == "" && conv.ParticipantID != "" {
			row.ParticipantID = conv.ParticipantID
			changed = true
		}
		if row.DMScope == "" {
			row.DMScope = dmScope
			changed = true
		}
		if changed {
			row.UpdatedAt = now
			next := cloneSessionIndex(s.index)
			next.Sessions[sessionKey] = row
			next.UpdatedAt = now
			if err := AtomicWriteJSON(s.sessionsPath, next, 0o644); err != nil {
				return "", false, err
			}
			s.index = next
		}
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
	next := cloneSessionIndex(s.index)
	next.Sessions[sessionKey] = model.SessionIndexRow{
		ActiveSessionID: sessionID,
		UpdatedAt:       now,
		ConversationID:  conv.ConversationID,
		ChannelType:     conv.ChannelType,
		ParticipantID:   conv.ParticipantID,
		DMScope:         dmScope,
	}
	next.UpdatedAt = now
	if err := AtomicWriteJSON(s.sessionsPath, next, 0o644); err != nil {
		return "", false, err
	}
	s.index = next
	return sessionID, true, nil
}

// UpdateProgress 刷新 session_key 对应会话的最新提交元信息并持久化索引。
func (s *SessionStore) UpdateProgress(sessionKey, sessionID, runID, commitID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.index.Sessions[sessionKey]
	row.ActiveSessionID = sessionID
	row.UpdatedAt = now
	if row.DMScope == "" {
		row.DMScope = defaultDMScope
	}
	if runID != "" {
		row.LastRunID = runID
	}
	if commitID != "" {
		row.LastCommitID = commitID
	}
	next := cloneSessionIndex(s.index)
	next.Sessions[sessionKey] = row
	next.UpdatedAt = now
	if err := AtomicWriteJSON(s.sessionsPath, next, 0o644); err != nil {
		return err
	}
	s.index = next
	return nil
}

// Get 返回指定 session_key 的索引记录。
func (s *SessionStore) Get(sessionKey string) (model.SessionIndexRow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.index.Sessions[sessionKey]
	return row, ok
}

// Snapshot 返回会话索引快照，用于查询接口筛选。
func (s *SessionStore) Snapshot() model.SessionIndex {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneSessionIndex(s.index)
}

func cloneSessionIndex(src model.SessionIndex) model.SessionIndex {
	out := model.SessionIndex{
		FormatVersion: src.FormatVersion,
		UpdatedAt:     src.UpdatedAt,
		Sessions:      make(map[string]model.SessionIndexRow, len(src.Sessions)),
	}
	for k, v := range src.Sessions {
		out.Sessions[k] = v
	}
	return out
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
