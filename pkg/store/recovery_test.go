package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestRecoverSessionIndexAcceptsLongJSONLLine(t *testing.T) {
	workspace := t.TempDir()
	if err := InitWorkspace(workspace); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	sessionID := "s_long_line"
	sessionKey := "sk:longline"
	runID := "run_long_line"
	commitID := "c_long_line"
	sessionPath := filepath.Join(workspace, "runtime", "sessions", sessionID+".jsonl")

	now := time.Now().UTC()
	header := model.SessionHeader{
		Type:          "header",
		SessionID:     sessionID,
		SessionKey:    sessionKey,
		CreatedAt:     now,
		FormatVersion: "1",
	}
	entry := model.SessionEntry{
		Type:    "assistant",
		EntryID: "e_long_line",
		RunID:   runID,
		Content: strings.Repeat("x", 11*1024*1024),
	}
	commit := model.SessionEntry{
		Type:    "commit",
		EntryID: "e_commit_long_line",
		RunID:   runID,
		Commit: &model.CommitMarker{
			CommitID:    commitID,
			RunID:       runID,
			EntryCount:  1,
			LastEntryID: entry.EntryID,
		},
	}
	if err := AppendJSONL(sessionPath, header, entry, commit); err != nil {
		t.Fatalf("append jsonl: %v", err)
	}

	idx, err := RecoverSessionIndex(workspace, model.SessionIndex{}, now.Add(time.Second))
	if err != nil {
		t.Fatalf("recover session index: %v", err)
	}
	row, ok := idx.Sessions[sessionKey]
	if !ok {
		t.Fatalf("session key %q missing in index", sessionKey)
	}
	if row.ActiveSessionID != sessionID {
		t.Fatalf("active session mismatch: got=%s want=%s", row.ActiveSessionID, sessionID)
	}
	if row.LastRunID != runID {
		t.Fatalf("last run mismatch: got=%s want=%s", row.LastRunID, runID)
	}
	if row.LastCommitID != commitID {
		t.Fatalf("last commit mismatch: got=%s want=%s", row.LastCommitID, commitID)
	}
}
