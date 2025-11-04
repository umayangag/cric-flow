-- Migration: Create player_consistency_data_fmt table and remove consistency columns from player table

CREATE TABLE IF NOT EXISTS player_consistency_data_fmt (
    id BIGSERIAL PRIMARY KEY,
    player_id BIGINT NOT NULL REFERENCES player(id),
    season_id BIGINT NOT NULL REFERENCES season(id),
    format_id BIGINT NOT NULL REFERENCES match_format(id),
    batting_consistency REAL,
    bowling_consistency REAL,
    UNIQUE(player_id, season_id, format_id)
);

CREATE INDEX IF NOT EXISTS idx_player_consistency_fmt_player_season_format
    ON player_consistency_data_fmt(player_id, season_id, format_id);

ALTER TABLE player
    DROP COLUMN IF EXISTS batting_consistency,
    DROP COLUMN IF EXISTS bowling_consistency;
