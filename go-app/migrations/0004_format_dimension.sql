-- Migration: add match format dimension and format-aware feature tables

-- 1) Canonical match formats
CREATE TABLE IF NOT EXISTS match_format (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(16) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL
);

-- Seed the initial set; safe on re-run
INSERT INTO match_format (code, name) VALUES
    ('TEST', 'Test'),
    ('ODI',  'One Day International'),
    ('T20',  'T20 (All)'),
    ('T20I', 'T20 International')
ON CONFLICT (code) DO NOTHING;

-- 2) Add format to match_details and index it
ALTER TABLE match_details
    ADD COLUMN IF NOT EXISTS format_id BIGINT;

-- Since we are starting from a fresh DB as per plan, enforce NOT NULL + FK immediately
ALTER TABLE match_details
    ALTER COLUMN format_id SET NOT NULL,
    ADD CONSTRAINT fk_match_details_format
        FOREIGN KEY (format_id) REFERENCES match_format(id);

CREATE INDEX IF NOT EXISTS idx_match_details_format ON match_details(format_id);

-- 3) Create format-aware feature tables (mirror existing style with id + UNIQUE composite key)

CREATE TABLE IF NOT EXISTS player_form_data_fmt (
    id BIGSERIAL PRIMARY KEY,
    player_id BIGINT NOT NULL REFERENCES player(id),
    season_id BIGINT NOT NULL REFERENCES season(id),
    format_id BIGINT NOT NULL REFERENCES match_format(id),
    batting_form REAL,
    bowling_form REAL,
    UNIQUE(player_id, season_id, format_id)
);
CREATE INDEX IF NOT EXISTS idx_player_form_fmt_player_season_format
    ON player_form_data_fmt(player_id, season_id, format_id);

CREATE TABLE IF NOT EXISTS player_venue_data_fmt (
    id BIGSERIAL PRIMARY KEY,
    player_id BIGINT NOT NULL REFERENCES player(id),
    venue_id BIGINT NOT NULL REFERENCES venue(id),
    format_id BIGINT NOT NULL REFERENCES match_format(id),
    batting_venue REAL,
    bowling_venue REAL,
    UNIQUE(player_id, venue_id, format_id)
);
CREATE INDEX IF NOT EXISTS idx_player_venue_fmt_player_venue_format
    ON player_venue_data_fmt(player_id, venue_id, format_id);

CREATE TABLE IF NOT EXISTS player_opposition_data_fmt (
    id BIGSERIAL PRIMARY KEY,
    player_id BIGINT NOT NULL REFERENCES player(id),
    opposition_id BIGINT NOT NULL REFERENCES opposition(id),
    format_id BIGINT NOT NULL REFERENCES match_format(id),
    batting_opposition REAL,
    bowling_opposition REAL,
    UNIQUE(player_id, opposition_id, format_id)
);
CREATE INDEX IF NOT EXISTS idx_player_opp_fmt_player_opp_format
    ON player_opposition_data_fmt(player_id, opposition_id, format_id);
