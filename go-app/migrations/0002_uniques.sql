-- Add unique constraints to support idempotent upserts

-- Weather: one row per (match_id, session)
DO $$ BEGIN
    ALTER TABLE weather_data
        ADD CONSTRAINT uq_weather_match_session UNIQUE (match_id, session);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Batting: one row per (match_id, player_id)
DO $$ BEGIN
    ALTER TABLE batting_data
        ADD CONSTRAINT uq_batting_match_player UNIQUE (match_id, player_id);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Bowling: one row per (match_id, player_id)
DO $$ BEGIN
    ALTER TABLE bowling_data
        ADD CONSTRAINT uq_bowling_match_player UNIQUE (match_id, player_id);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Fielding: one row per (match_id, player_id)
DO $$ BEGIN
    ALTER TABLE fielding_data
        ADD CONSTRAINT uq_fielding_match_player UNIQUE (match_id, player_id);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
