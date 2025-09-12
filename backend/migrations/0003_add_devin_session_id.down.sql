DROP INDEX IF EXISTS idx_game_specs_devin_session_id;
ALTER TABLE game_specs DROP COLUMN IF EXISTS devin_session_id;
