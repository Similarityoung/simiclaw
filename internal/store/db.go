package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	schemaVersion = 2
	dbFileName    = "app.db"
)

//go:embed schema.sql query.sql
var embeddedFiles embed.FS

type DB struct {
	path   string
	writer *sql.DB
	reader *sql.DB
}

func InitWorkspace(workspace string, args ...any) error {
	forceNewRuntime := false
	busyTimeout := DefaultBusyTimeout()
	if len(args) >= 1 {
		if v, ok := args[0].(bool); ok {
			forceNewRuntime = v
		}
	}
	if len(args) >= 2 {
		if v, ok := args[1].(time.Duration); ok && v > 0 {
			busyTimeout = v
		}
	}
	if err := rejectLegacyWorkspace(workspace, forceNewRuntime); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(workspace, "runtime", "native"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(workspace, "memory"), 0o755); err != nil {
		return err
	}

	db, err := openSQLite(filepath.Join(workspace, "runtime", dbFileName), busyTimeout)
	if err != nil {
		return err
	}
	defer db.Close()
	return applySchema(db)
}

func Open(workspace string, busyTimeout time.Duration) (*DB, error) {
	if err := rejectLegacyWorkspace(workspace, false); err != nil {
		return nil, err
	}
	path := filepath.Join(workspace, "runtime", dbFileName)
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}

	writer, err := openSQLite(path, busyTimeout)
	if err != nil {
		return nil, err
	}
	writer.SetMaxOpenConns(1)
	currentVersion, err := schemaUserVersion(writer)
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	if currentVersion < schemaVersion {
		if err := migrateSchema(writer, currentVersion); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := validateSchema(writer); err != nil {
		_ = writer.Close()
		return nil, err
	}

	reader, err := openSQLite(path, busyTimeout)
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	return &DB{path: path, writer: writer, reader: reader}, nil
}

func (db *DB) Close() error {
	var firstErr error
	if db.reader != nil {
		if err := db.reader.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if db.writer != nil {
		if err := db.writer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (db *DB) Writer() *sql.DB {
	return db.writer
}

func (db *DB) Reader() *sql.DB {
	return db.reader
}

func (db *DB) WithWriterTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := db.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (db *DB) CheckReadWrite(ctx context.Context) error {
	if err := db.reader.QueryRowContext(ctx, "SELECT 1").Scan(new(int)); err != nil {
		return err
	}
	tx, err := db.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	return tx.Rollback()
}

func DBPath(workspace string) string {
	return filepath.Join(workspace, "runtime", dbFileName)
}

func openSQLite(path string, busyTimeout time.Duration) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA foreign_keys=ON;",
		fmt.Sprintf("PRAGMA busy_timeout=%d;", busyTimeout.Milliseconds()),
	}
	for _, stmt := range pragmas {
		if _, err := db.Exec(stmt); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return db, nil
}

func applySchema(db *sql.DB) error {
	schema, err := readSchemaSQL()
	if err != nil {
		return err
	}
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	return validateSchema(db)
}

func readSchemaSQL() (string, error) {
	schema, err := embeddedFiles.ReadFile("schema.sql")
	if err != nil {
		return "", err
	}
	return string(schema), nil
}

func validateSchema(db *sql.DB) error {
	if err := validateSchemaVersion(db); err != nil {
		return err
	}
	return validateSchemaStructure(db)
}

func schemaUserVersion(db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func validateSchemaVersion(db *sql.DB) error {
	version, err := schemaUserVersion(db)
	if err != nil {
		return err
	}
	if version != schemaVersion {
		return fmt.Errorf("unsupported schema version %d", version)
	}
	return nil
}

func validateSchemaStructure(db *sql.DB) error {
	required := map[string][]string{
		"sessions":            {"session_key", "active_session_id", "conversation_id", "last_activity_at"},
		"conversation_scopes": {"tenant_id", "conversation_id", "dm_scope"},
		"messages":            {"message_id", "session_key", "visible", "meta_json"},
		"runs":                {"run_id", "event_id", "run_mode", "status", "started_at"},
		"events":              {"event_id", "session_key", "idempotency_key", "payload_hash", "status", "created_at", "updated_at"},
		"idempotency_keys":    {"key", "event_id", "payload_hash", "session_key"},
		"outbox":              {"outbox_id", "event_id", "session_key", "channel", "target_id", "status", "next_attempt_at"},
		"scheduled_jobs":      {"job_id", "kind", "status", "payload_json", "next_run_at"},
		"job_executions":      {"execution_id", "job_id", "status", "started_at"},
		"heartbeats":          {"worker_name", "beat_at", "status"},
	}
	for table, columns := range required {
		exists, err := tableExists(db, table)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("schema validation failed: missing table %s", table)
		}
		for _, column := range columns {
			ok, err := tableColumnExists(db, table, column)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("schema validation failed: missing column %s.%s", table, column)
			}
		}
	}
	return nil
}

func migrateSchema(db *sql.DB, currentVersion int) error {
	switch currentVersion {
	case schemaVersion:
		return nil
	case 1:
		return migrateV1ToV2(db)
	default:
		return fmt.Errorf("unsupported schema version %d", currentVersion)
	}
}

func migrateV1ToV2(db *sql.DB) error {
	outboxExists, err := tableExists(db, "outbox")
	if err != nil {
		return err
	}
	outboxHasChannel := false
	outboxHasTargetID := false
	if outboxExists {
		if outboxHasChannel, err = tableColumnExists(db, "outbox", "channel"); err != nil {
			return err
		}
		if outboxHasTargetID, err = tableColumnExists(db, "outbox", "target_id"); err != nil {
			return err
		}
	}

	schema, err := readSchemaSQL()
	if err != nil {
		return err
	}

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if outboxExists {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE outbox RENAME TO outbox_legacy_v1`); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return err
	}
	if outboxExists {
		channelExpr := "''"
		if outboxHasChannel {
			channelExpr = "COALESCE(channel, '')"
		}
		targetIDExpr := "''"
		if outboxHasTargetID {
			targetIDExpr = "COALESCE(target_id, '')"
		}
		copySQL := fmt.Sprintf(
			`INSERT INTO outbox (
				outbox_id, event_id, session_key, channel, target_id, body, status, next_attempt_at,
				locked_at, lock_owner, attempt_count, last_error, created_at, updated_at, sent_at
			)
			SELECT
				outbox_id, event_id, session_key, %s, %s, body, status, next_attempt_at,
				locked_at, lock_owner, attempt_count, last_error, created_at, updated_at, sent_at
			FROM outbox_legacy_v1`,
			channelExpr,
			targetIDExpr,
		)
		if _, err := tx.ExecContext(ctx, copySQL); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DROP TABLE outbox_legacy_v1`); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func tableExists(db *sql.DB, table string) (bool, error) {
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return name == table, nil
}

func tableColumnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal any
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func rejectLegacyWorkspace(workspace string, forceNewRuntime bool) error {
	legacyPaths := []string{
		filepath.Join(workspace, "runtime", "sessions.json"),
		filepath.Join(workspace, "runtime", "sessions"),
		filepath.Join(workspace, "runtime", "runs"),
		filepath.Join(workspace, "runtime", "approvals"),
		filepath.Join(workspace, "runtime", "idempotency"),
		filepath.Join(workspace, "runtime", "outbound_spool"),
		filepath.Join(workspace, "evolution"),
	}
	var found []string
	for _, p := range legacyPaths {
		if _, err := os.Stat(p); err == nil {
			found = append(found, p)
		}
	}
	if len(found) == 0 {
		return nil
	}
	if !forceNewRuntime {
		return fmt.Errorf("legacy workspace detected: %s", strings.Join(found, ", "))
	}
	for _, p := range found {
		if err := os.RemoveAll(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
