package store

import (
	"database/sql"
	"testing"
)

func TestInitWorkspaceAppliesSchemaV2(t *testing.T) {
	workspace := t.TempDir()
	if err := InitWorkspace(workspace, false, DefaultBusyTimeout()); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}
	db, err := Open(workspace, DefaultBusyTimeout())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	version, err := schemaUserVersion(db.Writer())
	if err != nil {
		t.Fatalf("schemaUserVersion: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("expected schema version %d, got %d", schemaVersion, version)
	}
	for _, column := range []string{"channel", "target_id"} {
		exists, err := tableColumnExists(db.Writer(), "outbox", column)
		if err != nil {
			t.Fatalf("tableColumnExists(%s): %v", column, err)
		}
		if !exists {
			t.Fatalf("expected outbox.%s to exist", column)
		}
	}
}

func TestOpenMigratesV1SchemaToV2(t *testing.T) {
	workspace := t.TempDir()
	path := DBPath(workspace)
	db, err := openSQLite(path, DefaultBusyTimeout())
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE sessions (
			session_key TEXT PRIMARY KEY,
			active_session_id TEXT NOT NULL,
			conversation_id TEXT NOT NULL,
			channel_type TEXT NOT NULL,
			participant_id TEXT NOT NULL DEFAULT '',
			dm_scope TEXT NOT NULL DEFAULT '',
			message_count INTEGER NOT NULL DEFAULT 0,
			prompt_tokens_total INTEGER NOT NULL DEFAULT 0,
			completion_tokens_total INTEGER NOT NULL DEFAULT 0,
			total_tokens_total INTEGER NOT NULL DEFAULT 0,
			last_model TEXT NOT NULL DEFAULT '',
			last_run_id TEXT NOT NULL DEFAULT '',
			last_activity_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE messages (
			fts_rowid INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id TEXT NOT NULL UNIQUE,
			session_key TEXT NOT NULL,
			session_id TEXT NOT NULL,
			run_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			visible INTEGER NOT NULL DEFAULT 1,
			tool_call_id TEXT NOT NULL DEFAULT '',
			tool_name TEXT NOT NULL DEFAULT '',
			tool_args_json TEXT NOT NULL DEFAULT '',
			tool_result_json TEXT NOT NULL DEFAULT '',
			meta_json TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);
		CREATE VIRTUAL TABLE messages_fts USING fts5(content, content='messages', content_rowid='fts_rowid');
		CREATE TABLE runs (
			run_id TEXT PRIMARY KEY,
			event_id TEXT NOT NULL,
			session_key TEXT NOT NULL,
			session_id TEXT NOT NULL,
			run_mode TEXT NOT NULL,
			status TEXT NOT NULL,
			provider TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			latency_ms INTEGER NOT NULL DEFAULT 0,
			finish_reason TEXT NOT NULL DEFAULT '',
			raw_finish_reason TEXT NOT NULL DEFAULT '',
			provider_request_id TEXT NOT NULL DEFAULT '',
			output_text TEXT NOT NULL DEFAULT '',
			tool_calls_json TEXT NOT NULL DEFAULT '',
			diagnostics_json TEXT NOT NULL DEFAULT '',
			error_code TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE events (
			event_id TEXT PRIMARY KEY,
			source TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			conversation_id TEXT NOT NULL,
			channel_type TEXT NOT NULL,
			participant_id TEXT NOT NULL DEFAULT '',
			session_key TEXT NOT NULL,
			session_id TEXT NOT NULL,
			idempotency_key TEXT NOT NULL UNIQUE,
			payload_type TEXT NOT NULL,
			payload_text TEXT NOT NULL DEFAULT '',
			payload_json TEXT NOT NULL,
			payload_hash TEXT NOT NULL,
			status TEXT NOT NULL,
			run_id TEXT NOT NULL DEFAULT '',
			run_mode TEXT NOT NULL DEFAULT 'NORMAL',
			assistant_reply TEXT NOT NULL DEFAULT '',
			outbox_id TEXT NOT NULL DEFAULT '',
			outbox_status TEXT NOT NULL DEFAULT '',
			processing_started_at TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			provider_request_id TEXT NOT NULL DEFAULT '',
			error_code TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE idempotency_keys (
			key TEXT PRIMARY KEY,
			event_id TEXT NOT NULL,
			payload_hash TEXT NOT NULL,
			session_key TEXT NOT NULL,
			session_id TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE TABLE outbox (
			outbox_id TEXT PRIMARY KEY,
			event_id TEXT NOT NULL,
			session_key TEXT NOT NULL,
			body TEXT NOT NULL,
			status TEXT NOT NULL,
			next_attempt_at TEXT NOT NULL,
			locked_at TEXT NOT NULL DEFAULT '',
			lock_owner TEXT NOT NULL DEFAULT '',
			attempt_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			sent_at TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE scheduled_jobs (
			job_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			kind TEXT NOT NULL,
			status TEXT NOT NULL,
			payload_json TEXT NOT NULL DEFAULT '{}',
			next_run_at TEXT NOT NULL,
			locked_at TEXT NOT NULL DEFAULT '',
			lock_owner TEXT NOT NULL DEFAULT '',
			attempt_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE job_executions (
			execution_id TEXT PRIMARY KEY,
			job_id TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT '',
			created_event_id TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE heartbeats (
			worker_name TEXT PRIMARY KEY,
			beat_at TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'alive',
			details TEXT NOT NULL DEFAULT ''
		);
		INSERT INTO outbox (
			outbox_id, event_id, session_key, body, status, next_attempt_at, created_at, updated_at
		) VALUES ('out_1', 'evt_1', 'local:dm:u1', 'hello', 'pending', '2026-03-10T00:00:00Z', '2026-03-10T00:00:00Z', '2026-03-10T00:00:00Z');
		PRAGMA user_version = 1;
	`); err != nil {
		t.Fatalf("seed old schema: %v", err)
	}
	_ = db.Close()

	opened, err := Open(workspace, DefaultBusyTimeout())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer opened.Close()

	version, err := schemaUserVersion(opened.Writer())
	if err != nil {
		t.Fatalf("schemaUserVersion: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("expected schema version %d, got %d", schemaVersion, version)
	}
	for _, column := range []string{"channel", "target_id"} {
		exists, err := tableColumnExists(opened.Writer(), "outbox", column)
		if err != nil {
			t.Fatalf("tableColumnExists(%s): %v", column, err)
		}
		if !exists {
			t.Fatalf("expected outbox.%s after migration", column)
		}
	}
	var channel, targetID string
	if err := opened.Reader().QueryRow(`SELECT channel, target_id FROM outbox WHERE outbox_id = 'out_1'`).Scan(&channel, &targetID); err != nil {
		t.Fatalf("read migrated outbox row: %v", err)
	}
	if channel != "" || targetID != "" {
		t.Fatalf("expected migrated routing columns to backfill empty strings, got channel=%q target_id=%q", channel, targetID)
	}
	if !tablePresentSQL(t, opened.Writer(), "conversation_scopes") {
		t.Fatalf("expected conversation_scopes after migration")
	}
}

func TestOpenRejectsInvalidV2Schema(t *testing.T) {
	workspace := t.TempDir()
	path := DBPath(workspace)
	db, err := openSQLite(path, DefaultBusyTimeout())
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE outbox (
			outbox_id TEXT PRIMARY KEY,
			event_id TEXT NOT NULL,
			session_key TEXT NOT NULL,
			channel TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL,
			status TEXT NOT NULL,
			next_attempt_at TEXT NOT NULL,
			locked_at TEXT NOT NULL DEFAULT '',
			lock_owner TEXT NOT NULL DEFAULT '',
			attempt_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			sent_at TEXT NOT NULL DEFAULT ''
		);
		PRAGMA user_version = 2;
	`); err != nil {
		t.Fatalf("seed broken schema: %v", err)
	}
	_ = db.Close()

	if _, err := Open(workspace, DefaultBusyTimeout()); err == nil {
		t.Fatalf("expected Open to reject invalid v2 schema")
	}
}

func tablePresentSQL(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("query sqlite_master for %s: %v", table, err)
	}
	return name == table
}
