package persistence

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type sessionLine struct {
	Type       string              `json:"type"`
	EntryID    string              `json:"entry_id,omitempty"`
	RunID      string              `json:"run_id,omitempty"`
	SessionID  string              `json:"session_id,omitempty"`
	SessionKey string              `json:"session_key,omitempty"`
	Commit     *model.CommitMarker `json:"commit,omitempty"`
}

type sessionRecoveryState struct {
	SessionKey   string
	LastCommitID string
	LastRunID    string
}

// RecoverSessionIndex 对 runtime/sessions/*.jsonl 做尾部修复并重建/刷新 sessions.json。
func RecoverSessionIndex(workspace string, existing model.SessionIndex, now time.Time) (model.SessionIndex, error) {
	if existing.FormatVersion == "" {
		existing.FormatVersion = "1"
	}
	if existing.Sessions == nil {
		existing.Sessions = map[string]model.SessionIndexRow{}
	}

	sessionsDir := filepath.Join(workspace, "runtime", "sessions")
	files, err := filepath.Glob(filepath.Join(sessionsDir, "*.jsonl"))
	if err != nil {
		return model.SessionIndex{}, err
	}
	sort.Strings(files)

	next := model.SessionIndex{
		FormatVersion: existing.FormatVersion,
		UpdatedAt:     now,
		Sessions:      map[string]model.SessionIndexRow{},
	}
	for k, v := range existing.Sessions {
		next.Sessions[k] = v
	}

	for _, p := range files {
		sessionID := strings.TrimSuffix(filepath.Base(p), ".jsonl")
		state, err := sanitizeSessionFile(p)
		if err != nil {
			return model.SessionIndex{}, err
		}
		sessionKey := state.SessionKey
		if sessionKey == "" {
			sessionKey = "unknown:" + sessionID
		}
		row := next.Sessions[sessionKey]
		row.ActiveSessionID = sessionID
		row.UpdatedAt = now
		if row.DMScope == "" {
			row.DMScope = defaultDMScope
		}
		if state.LastCommitID != "" {
			row.LastCommitID = state.LastCommitID
		}
		if state.LastRunID != "" {
			row.LastRunID = state.LastRunID
		}
		next.Sessions[sessionKey] = row
	}

	sessionsPath := filepath.Join(workspace, "runtime", "sessions.json")
	if err := AtomicWriteJSON(sessionsPath, next, 0o644); err != nil {
		return model.SessionIndex{}, err
	}
	return next, nil
}

func sanitizeSessionFile(path string) (sessionRecoveryState, error) {
	f, err := os.Open(path)
	if err != nil {
		return sessionRecoveryState{}, err
	}
	defer f.Close()

	reader := bufio.NewReaderSize(f, 64*1024)

	rawLines := make([][]byte, 0, 64)
	lines := make([]sessionLine, 0, 64)
	parseFailed := false
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return sessionRecoveryState{}, err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}
		var parsed sessionLine
		if err := json.Unmarshal(line, &parsed); err != nil {
			parseFailed = true
			break
		}
		raw := make([]byte, len(line))
		copy(raw, line)
		rawLines = append(rawLines, raw)
		lines = append(lines, parsed)
		if errors.Is(err, io.EOF) {
			break
		}
	}
	if len(lines) == 0 {
		return sessionRecoveryState{}, nil
	}

	state := sessionRecoveryState{}
	start := 0
	if lines[0].Type == "header" {
		start = 1
		state.SessionKey = lines[0].SessionKey
	}

	validEnd := start
	pos := start
	for pos < len(lines) {
		commitIdx := -1
		for i := pos; i < len(lines); i++ {
			if lines[i].Type == "commit" {
				commitIdx = i
				break
			}
		}
		if commitIdx == -1 {
			break
		}

		batch := lines[pos:commitIdx]
		commitLine := lines[commitIdx]
		if !isValidCommittedBatch(batch, commitLine) {
			break
		}
		validEnd = commitIdx + 1
		state.LastCommitID = commitLine.Commit.CommitID
		if commitLine.RunID != "" {
			state.LastRunID = commitLine.RunID
		} else {
			state.LastRunID = commitLine.Commit.RunID
		}
		pos = commitIdx + 1
	}

	if parseFailed || validEnd < len(lines) {
		var b bytes.Buffer
		for i := 0; i < validEnd; i++ {
			b.Write(rawLines[i])
			b.WriteByte('\n')
		}
		if err := AtomicWriteFile(path, b.Bytes(), 0o644); err != nil {
			return sessionRecoveryState{}, err
		}
	}

	return state, nil
}

func isValidCommittedBatch(batch []sessionLine, commit sessionLine) bool {
	if commit.Type != "commit" || commit.Commit == nil {
		return false
	}
	if len(batch) != commit.Commit.EntryCount {
		return false
	}

	runID := commit.RunID
	if runID == "" {
		runID = commit.Commit.RunID
	}
	if runID == "" {
		return false
	}
	if commit.Commit.RunID != "" && commit.Commit.RunID != runID {
		return false
	}

	if len(batch) > 0 {
		lastEntryID := batch[len(batch)-1].EntryID
		if lastEntryID == "" || commit.Commit.LastEntryID != lastEntryID {
			return false
		}
	}
	for _, e := range batch {
		if e.Type == "commit" {
			return false
		}
		if e.EntryID == "" || e.RunID == "" {
			return false
		}
		if e.RunID != runID {
			return false
		}
	}
	return true
}
