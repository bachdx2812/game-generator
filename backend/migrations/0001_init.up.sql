CREATE TABLE IF NOT EXISTS game_specs (
    id UUID PRIMARY KEY,
    title TEXT NOT NULL,
    brief TEXT NOT NULL,
    spec_markdown TEXT NOT NULL,
    spec_json JSONB NOT NULL,
    spec_hash TEXT NOT NULL UNIQUE,
    genre TEXT,
    duration_sec INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by UUID
);

CREATE TABLE IF NOT EXISTS gen_spec_jobs (
    id UUID PRIMARY KEY,
    status TEXT NOT NULL CHECK (status IN ('QUEUED','RUNNING','DUPLICATE','COMPLETED','FAILED')),
    brief TEXT NOT NULL,
    result_spec_id UUID NULL REFERENCES game_specs(id) ON DELETE SET NULL,
    duplicate_of UUID[] NULL,
    score_similarity NUMERIC NULL,
    error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ NULL,
    finished_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_game_specs_created_at ON game_specs(created_at DESC);
