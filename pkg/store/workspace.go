package store

import (
	"os"
	"path/filepath"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func InitWorkspace(workspace string) error {
	paths := []string{
		filepath.Join(workspace, "runtime"),
		filepath.Join(workspace, "runtime", "sessions"),
		filepath.Join(workspace, "runtime", "runs"),
		filepath.Join(workspace, "runtime", "idempotency"),
		filepath.Join(workspace, "runtime", "outbound_spool"),
		filepath.Join(workspace, "runtime", "native"),
		filepath.Join(workspace, "runtime", "events"),
		filepath.Join(workspace, "tests"),
	}
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return err
		}
	}

	sessionsPath := filepath.Join(workspace, "runtime", "sessions.json")
	if _, err := os.Stat(sessionsPath); os.IsNotExist(err) {
		idx := model.SessionIndex{
			FormatVersion: "1",
			UpdatedAt:     time.Now().UTC(),
			Sessions:      map[string]model.SessionIndexRow{},
		}
		if err := AtomicWriteJSON(sessionsPath, idx, 0o644); err != nil {
			return err
		}
	}

	jobsPath := filepath.Join(workspace, "runtime", "cron", "jobs.json")
	if err := os.MkdirAll(filepath.Dir(jobsPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(jobsPath); os.IsNotExist(err) {
		if err := AtomicWriteFile(jobsPath, []byte("{\n  \"jobs\": []\n}\n"), 0o644); err != nil {
			return err
		}
	}

	eventsPath := filepath.Join(workspace, "runtime", "events", "events.json")
	if _, err := os.Stat(eventsPath); os.IsNotExist(err) {
		if err := AtomicWriteFile(eventsPath, []byte("{\n  \"events\": {}\n}\n"), 0o644); err != nil {
			return err
		}
	}

	for _, f := range []string{"inbound_keys.jsonl", "outbound_keys.jsonl", "action_keys.jsonl"} {
		p := filepath.Join(workspace, "runtime", "idempotency", f)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			if err := AtomicWriteFile(p, []byte(""), 0o644); err != nil {
				return err
			}
		}
	}

	return nil
}
