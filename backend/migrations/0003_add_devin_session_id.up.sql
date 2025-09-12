ALTER TABLE game_specs ADD COLUMN devin_session_id TEXT NULL;
CREATE INDEX IF NOT EXISTS idx_game_specs_devin_session_id ON game_specs(devin_session_id);
