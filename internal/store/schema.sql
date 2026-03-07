PRAGMA user_version = 1;

CREATE TABLE IF NOT EXISTS sessions (
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

CREATE TABLE IF NOT EXISTS messages (
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

CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    content,
    content='messages',
    content_rowid='fts_rowid'
);

CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
    INSERT INTO messages_fts(rowid, content) VALUES (new.fts_rowid, new.content);
END;

CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, content) VALUES ('delete', old.fts_rowid, old.content);
END;

CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, content) VALUES ('delete', old.fts_rowid, old.content);
    INSERT INTO messages_fts(rowid, content) VALUES (new.fts_rowid, new.content);
END;

CREATE TABLE IF NOT EXISTS runs (
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

CREATE TABLE IF NOT EXISTS events (
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

CREATE TABLE IF NOT EXISTS idempotency_keys (
    key TEXT PRIMARY KEY,
    event_id TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    session_key TEXT NOT NULL,
    session_id TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS outbox (
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

CREATE TABLE IF NOT EXISTS scheduled_jobs (
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

CREATE TABLE IF NOT EXISTS job_executions (
    execution_id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_event_id TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS heartbeats (
    worker_name TEXT PRIMARY KEY,
    beat_at TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'alive',
    details TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_events_status_updated_at ON events(status, updated_at, event_id);
CREATE INDEX IF NOT EXISTS idx_runs_session_started_at ON runs(session_id, started_at);
CREATE INDEX IF NOT EXISTS idx_runs_started_at_id ON runs(started_at, run_id);
CREATE INDEX IF NOT EXISTS idx_messages_session_created_at ON messages(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_outbox_status_next_attempt_at ON outbox(status, next_attempt_at);
CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_status_next_run_at ON scheduled_jobs(status, next_run_at);
