-- Drop indexes
DROP INDEX IF EXISTS idx_game_spec_states_created_at;
DROP INDEX IF EXISTS idx_game_spec_states_game_spec_id;
DROP INDEX IF EXISTS idx_game_specs_state;

-- Drop game_spec_states table
DROP TABLE IF EXISTS game_spec_states;

-- Remove state column from game_specs
ALTER TABLE game_specs DROP COLUMN IF EXISTS state;
