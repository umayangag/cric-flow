-- Add inning_number to fielding_data so fielding stats are tracked per inning,
-- matching the pattern already used by batting_data and bowling_data.

ALTER TABLE fielding_data ADD COLUMN inning_number SMALLINT NOT NULL DEFAULT 1;

-- Drop the old unique constraint (match_id, player_id) and replace with (match_id, inning_number, player_id).
ALTER TABLE fielding_data DROP CONSTRAINT IF EXISTS fielding_data_match_id_player_id_key;
ALTER TABLE fielding_data ADD CONSTRAINT fielding_data_match_id_inning_number_player_id_key UNIQUE (match_id, inning_number, player_id);

CREATE INDEX IF NOT EXISTS idx_fielding_match_inning ON fielding_data(match_id, inning_number);
