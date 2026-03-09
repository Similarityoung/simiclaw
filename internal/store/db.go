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
	schemaVersion = 1
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
	if err := validateSchemaVersion(writer); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := ensureSchemaFeatures(writer); err != nil {
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
	schema, err := embeddedFiles.ReadFile("schema.sql")
	if err != nil {
		return err
	}
	if _, err := db.Exec(string(schema)); err != nil {
		return err
	}
	if err := validateSchemaVersion(db); err != nil {
		return err
	}
	return ensureSchemaFeatures(db)
}

func validateSchemaVersion(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		return err
	}
	if version != schemaVersion {
		return fmt.Errorf("unsupported schema version %d", version)
	}
	return nil
}

func ensureSchemaFeatures(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS conversation_scopes (
		tenant_id TEXT NOT NULL,
		conversation_id TEXT NOT NULL,
		channel_type TEXT NOT NULL,
		participant_id TEXT NOT NULL DEFAULT '',
		dm_scope TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL,
		PRIMARY KEY (tenant_id, conversation_id, channel_type, participant_id)
	)`); err != nil {
		return err
	}

	features := []struct {
		table  string
		column string
		ddl    string
	}{
		{table: "outbox", column: "channel", ddl: "ALTER TABLE outbox ADD COLUMN channel TEXT NOT NULL DEFAULT ''"},
		{table: "outbox", column: "target_id", ddl: "ALTER TABLE outbox ADD COLUMN target_id TEXT NOT NULL DEFAULT ''"},
	}
	for _, feature := range features {
		exists, err := tableColumnExists(db, feature.table, feature.column)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.Exec(feature.ddl); err != nil {
			return err
		}
	}
	return nil
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
