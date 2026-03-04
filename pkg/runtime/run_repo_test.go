package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestRunRepoListSkipsCorruptedFiles(t *testing.T) {
	workspace := t.TempDir()
	runsDir := filepath.Join(workspace, "runtime", "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		t.Fatalf("mkdir runs dir: %v", err)
	}

	valid := model.RunTrace{
		RunID:      "run_valid",
		EventID:    "evt_1",
		SessionKey: "sk:1",
		SessionID:  "s_1",
		RunMode:    model.RunModeNormal,
		StartedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
	}
	if err := os.WriteFile(filepath.Join(runsDir, "run_valid.json"), []byte(`{
  "run_id": "run_valid",
  "event_id": "evt_1",
  "session_key": "sk:1",
  "session_id": "s_1",
  "run_mode": "NORMAL",
  "actions": [],
  "started_at": "`+valid.StartedAt.Format(time.RFC3339Nano)+`",
  "finished_at": "`+valid.FinishedAt.Format(time.RFC3339Nano)+`"
}`), 0o644); err != nil {
		t.Fatalf("write valid run: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, "run_bad.json"), []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("write corrupted run: %v", err)
	}

	repo := NewRunRepo(workspace)
	runs, err := repo.List()
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 valid run after skipping corrupted files, got %d", len(runs))
	}
	if runs[0].RunID != "run_valid" {
		t.Fatalf("unexpected run_id: got=%s want=run_valid", runs[0].RunID)
	}
}
