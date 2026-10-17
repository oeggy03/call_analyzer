CREATE TABLE IF NOT EXISTS lessons (
    id TEXT PRIMARY KEY NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    started_at INTEGER NOT NULL,
    ended_at INTEGER,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS transcript_segments (
    id TEXT PRIMARY KEY NOT NULL,
    lesson_id TEXT NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,
    start_ms INTEGER NOT NULL,
    end_ms INTEGER NOT NULL,
    text TEXT NOT NULL,
    simplified_text TEXT NOT NULL DEFAULT '',
    traditional_text TEXT NOT NULL DEFAULT '',
    reading TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    confidence REAL NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    CHECK (end_ms >= start_ms)
);

CREATE INDEX IF NOT EXISTS idx_transcript_segments_lesson_time
    ON transcript_segments(lesson_id, start_ms, end_ms);

CREATE TABLE IF NOT EXISTS vocabulary_entries (
    id TEXT PRIMARY KEY NOT NULL,
    simplified TEXT NOT NULL,
    traditional TEXT NOT NULL,
    reading TEXT NOT NULL,
    marked_pinyin TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('candidate', 'confirmed', 'rejected', 'merged')),
    provenance TEXT NOT NULL,
    confidence REAL NOT NULL DEFAULT 0,
    teaching_cue REAL NOT NULL DEFAULT 0,
    manual INTEGER NOT NULL DEFAULT 0 CHECK (manual IN (0, 1)),
    priority TEXT NOT NULL DEFAULT '',
    merged_into_id TEXT REFERENCES vocabulary_entries(id) ON DELETE SET NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_vocabulary_dedup
    ON vocabulary_entries(simplified, traditional, reading);
CREATE INDEX IF NOT EXISTS idx_vocabulary_status_updated
    ON vocabulary_entries(status, updated_at DESC);

CREATE TABLE IF NOT EXISTS senses (
    id TEXT PRIMARY KEY NOT NULL,
    entry_id TEXT NOT NULL REFERENCES vocabulary_entries(id) ON DELETE CASCADE,
    gloss TEXT NOT NULL,
    part_of_speech TEXT NOT NULL DEFAULT '',
    classifier TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS examples (
    id TEXT PRIMARY KEY NOT NULL,
    entry_id TEXT NOT NULL REFERENCES vocabulary_entries(id) ON DELETE CASCADE,
    simplified TEXT NOT NULL,
    traditional TEXT NOT NULL DEFAULT '',
    reading TEXT NOT NULL DEFAULT '',
    translation TEXT NOT NULL DEFAULT '',
    provenance TEXT NOT NULL,
    generated INTEGER NOT NULL DEFAULT 0 CHECK (generated IN (0, 1))
);

CREATE TABLE IF NOT EXISTS observations (
    id TEXT PRIMARY KEY NOT NULL,
    lesson_id TEXT NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,
    segment_id TEXT REFERENCES transcript_segments(id) ON DELETE SET NULL,
    entry_id TEXT NOT NULL REFERENCES vocabulary_entries(id) ON DELETE CASCADE,
    evidence_text TEXT NOT NULL,
    start_ms INTEGER NOT NULL DEFAULT 0,
    end_ms INTEGER NOT NULL DEFAULT 0,
    confidence REAL NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_observations_lesson
    ON observations(lesson_id, created_at);
CREATE INDEX IF NOT EXISTS idx_observations_entry
    ON observations(entry_id, created_at);

CREATE TABLE IF NOT EXISTS tags (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS entry_tags (
    entry_id TEXT NOT NULL REFERENCES vocabulary_entries(id) ON DELETE CASCADE,
    tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (entry_id, tag_id)
);

CREATE TABLE IF NOT EXISTS study_state (
    entry_id TEXT PRIMARY KEY NOT NULL REFERENCES vocabulary_entries(id) ON DELETE CASCADE,
    due_at INTEGER,
    interval_days REAL NOT NULL DEFAULT 0,
    ease REAL NOT NULL DEFAULT 2.5,
    repetitions INTEGER NOT NULL DEFAULT 0,
    lapses INTEGER NOT NULL DEFAULT 0,
    last_reviewed_at INTEGER
);

CREATE TABLE IF NOT EXISTS review_events (
    id TEXT PRIMARY KEY NOT NULL,
    entry_id TEXT NOT NULL REFERENCES vocabulary_entries(id) ON DELETE CASCADE,
    rating INTEGER NOT NULL CHECK (rating >= 0 AND rating <= 5),
    reviewed_at INTEGER NOT NULL,
    metadata TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS outbox (
    id TEXT PRIMARY KEY NOT NULL,
    event_type TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    delivered_at INTEGER,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_outbox_pending
    ON outbox(delivered_at, created_at);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY NOT NULL,
    value_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS request_usage (
    id TEXT PRIMARY KEY NOT NULL,
    session_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    cost REAL NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_request_usage_session
    ON request_usage(session_id, created_at);
