package store

import (
	"database/sql"
	"errors"
	"testing"
)

func TestSchemaHelpersEdgeCases(t *testing.T) {
	workspace := t.TempDir()
	if _, err := Open(workspace, DefaultBusyTimeout()); err == nil {
		t.Fatalf("expected Open to fail when database is missing")
	}

	db, err := openSQLite(DBPath(workspace), DefaultBusyTimeout())
	if err != nil {
		t.Fatalf("openSQLite: %v", err)
	}
	defer db.Close()

	if err := validateSchemaVersion(db); err == nil {
		t.Fatalf("expected validateSchemaVersion to reject version 0")
	}
	if err := validateSchemaStructure(db); err == nil {
		t.Fatalf("expected validateSchemaStructure to reject empty schema")
	}
	if err := migrateSchema(db, 99); err == nil {
		t.Fatalf("expected migrateSchema to reject unsupported version")
	}

	exists, err := tableExists(db, "missing_table")
	if err != nil {
		t.Fatalf("tableExists: %v", err)
	}
	if exists {
		t.Fatalf("expected missing table to report false")
	}

	if _, err := db.Exec(`CREATE TABLE demo (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create demo table: %v", err)
	}
	hasColumn, err := tableColumnExists(db, "demo", "missing_column")
	if err != nil {
		t.Fatalf("tableColumnExists: %v", err)
	}
	if hasColumn {
		t.Fatalf("expected missing column to report false")
	}
}

func TestOpenAddsOutboxRoutingColumns(t *testing.T) {
	workspace := t.TempDir()
	path := DBPath(workspace)
	db, err := openSQLite(path, DefaultBusyTimeout())
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`
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
		PRAGMA user_version = 1;
	`); err != nil {
		t.Fatalf("seed old schema: %v", err)
	}
	_ = db.Close()

	opened, err := Open(workspace, DefaultBusyTimeout())
	if err != nil {
		t.Fatalf("open db with migration: %v", err)
	}
	defer opened.Close()

	for _, column := range []string{"channel", "target_id"} {
		exists, err := tableColumnExists(opened.Writer(), "outbox", column)
		if err != nil {
			t.Fatalf("tableColumnExists(%s): %v", column, err)
		}
		if !exists {
			t.Fatalf("expected outbox.%s to exist after migration", column)
		}
	}
	if !tablePresent(t, opened.Writer(), "conversation_scopes") {
		t.Fatalf("expected conversation_scopes to exist after migration")
	}
}

func tablePresent(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		t.Fatalf("query sqlite_master for %s: %v", table, err)
	}
	return name == table
}
