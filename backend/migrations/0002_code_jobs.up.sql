CREATE TABLE IF NOT EXISTS code_jobs (
    id UUID PRIMARY KEY,
    game_spec_id UUID REFERENCES game_specs(id),
    game_spec JSONB,
    output_path TEXT,
    status TEXT NOT NULL DEFAULT 'queued',
    progress INTEGER DEFAULT 0,
    artifact_url TEXT,
    error TEXT,
    logs JSONB DEFAULT '[]'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX idx_code_jobs_status ON code_jobs(status);
CREATE INDEX idx_code_jobs_created_at ON code_jobs(created_at);
