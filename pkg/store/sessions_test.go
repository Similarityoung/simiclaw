package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestResolveSessionWriteFailureKeepsMemoryIndexUnchanged(t *testing.T) {
	workspace := t.TempDir()
	failPath := filepath.Join(workspace, "runtime")
	if err := InitWorkspace(workspace); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	now := time.Now().UTC()
	initialRow := model.SessionIndexRow{
		ActiveSessionID: "s_existing",
		UpdatedAt:       now,
	}
	s := &SessionStore{
		workspace:    workspace,
		sessionsPath: failPath,
		index: model.SessionIndex{
			FormatVersion: "1",
			UpdatedAt:     now,
			Sessions: map[string]model.SessionIndexRow{
				"sk:1": initialRow,
			},
		},
	}

	_, _, err := s.ResolveSession("sk:1", model.Conversation{
		ConversationID: "conv_1",
		ChannelType:    "dm",
		ParticipantID:  "u1",
	}, "default", now.Add(time.Second))
	if err == nil {
		t.Fatalf("expected resolve session to fail when sessionsPath is a directory")
	}

	row := s.index.Sessions["sk:1"]
	if row.ActiveSessionID != initialRow.ActiveSessionID {
		t.Fatalf("active session id should stay unchanged: got=%s want=%s", row.ActiveSessionID, initialRow.ActiveSessionID)
	}
	if row.ConversationID != "" || row.ChannelType != "" || row.ParticipantID != "" || row.DMScope != "" {
		t.Fatalf("row metadata should not change on write failure: %+v", row)
	}
	if !row.UpdatedAt.Equal(initialRow.UpdatedAt) {
		t.Fatalf("row updated_at should stay unchanged: got=%s want=%s", row.UpdatedAt, initialRow.UpdatedAt)
	}
	if !s.index.UpdatedAt.Equal(now) {
		t.Fatalf("index updated_at should stay unchanged: got=%s want=%s", s.index.UpdatedAt, now)
	}
}

func TestUpdateProgressWriteFailureKeepsMemoryIndexUnchanged(t *testing.T) {
	workspace := t.TempDir()
	failPath := filepath.Join(workspace, "runtime")
	if err := InitWorkspace(workspace); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	now := time.Now().UTC()
	initialRow := model.SessionIndexRow{
		ActiveSessionID: "s_old",
		UpdatedAt:       now,
		DMScope:         "default",
		LastRunID:       "run_old",
		LastCommitID:    "c_old",
	}
	s := &SessionStore{
		workspace:    workspace,
		sessionsPath: failPath,
		index: model.SessionIndex{
			FormatVersion: "1",
			UpdatedAt:     now,
			Sessions: map[string]model.SessionIndexRow{
				"sk:2": initialRow,
			},
		},
	}

	err := s.UpdateProgress("sk:2", "s_new", "run_new", "c_new", now.Add(time.Second))
	if err == nil {
		t.Fatalf("expected update progress to fail when sessionsPath is a directory")
	}

	row := s.index.Sessions["sk:2"]
	if row.ActiveSessionID != initialRow.ActiveSessionID {
		t.Fatalf("active session id should stay unchanged: got=%s want=%s", row.ActiveSessionID, initialRow.ActiveSessionID)
	}
	if row.LastRunID != initialRow.LastRunID || row.LastCommitID != initialRow.LastCommitID {
		t.Fatalf("progress fields should stay unchanged: got=%+v want=%+v", row, initialRow)
	}
	if !row.UpdatedAt.Equal(initialRow.UpdatedAt) {
		t.Fatalf("row updated_at should stay unchanged: got=%s want=%s", row.UpdatedAt, initialRow.UpdatedAt)
	}
	if !s.index.UpdatedAt.Equal(now) {
		t.Fatalf("index updated_at should stay unchanged: got=%s want=%s", s.index.UpdatedAt, now)
	}
}
