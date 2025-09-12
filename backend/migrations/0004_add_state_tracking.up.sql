-- Add state field to game_specs table
ALTER TABLE game_specs ADD COLUMN state TEXT NOT NULL DEFAULT 'creating';

-- Create game_spec_states table for tracking state changes
CREATE TABLE IF NOT EXISTS game_spec_states (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_spec_id UUID NOT NULL REFERENCES game_specs(id) ON DELETE CASCADE,
    state_before TEXT,
    state_after TEXT NOT NULL,
    detail TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_game_specs_state ON game_specs(state);
CREATE INDEX IF NOT EXISTS idx_game_spec_states_game_spec_id ON game_spec_states(game_spec_id);
CREATE INDEX IF NOT EXISTS idx_game_spec_states_created_at ON game_spec_states(created_at DESC);
