-- Migration: create time-indexed (as-of) feature snapshot tables

-- Player form snapshots as of a cutoff date (per-format)
CREATE TABLE IF NOT EXISTS player_form_asof (
    id BIGSERIAL PRIMARY KEY,
    player_id BIGINT NOT NULL REFERENCES player(id),
    as_of_date DATE NOT NULL,
    format_id BIGINT NOT NULL REFERENCES match_format(id),
    bat_form REAL,
    bowl_form REAL,
    n_samples_bat REAL,
    n_samples_bowl REAL,
    window_spec TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(player_id, as_of_date, format_id)
);
CREATE INDEX IF NOT EXISTS idx_player_form_asof_player_date_format
    ON player_form_asof(player_id, as_of_date, format_id);

-- Player consistency snapshots as of a cutoff date (per-format)
CREATE TABLE IF NOT EXISTS player_consistency_asof (
    id BIGSERIAL PRIMARY KEY,
    player_id BIGINT NOT NULL REFERENCES player(id),
    as_of_date DATE NOT NULL,
    format_id BIGINT NOT NULL REFERENCES match_format(id),
    bat_consistency REAL,
    bowl_consistency REAL,
    n_samples_bat INTEGER,
    n_samples_bowl INTEGER,
    window_spec TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(player_id, as_of_date, format_id)
);
CREATE INDEX IF NOT EXISTS idx_player_consistency_asof_player_date_format
    ON player_consistency_asof(player_id, as_of_date, format_id);

-- Player vs opposition snapshots as of a cutoff date (per-format)
CREATE TABLE IF NOT EXISTS player_vs_opposition_asof (
    id BIGSERIAL PRIMARY KEY,
    player_id BIGINT NOT NULL REFERENCES player(id),
    opposition_id BIGINT NOT NULL REFERENCES opposition(id),
    as_of_date DATE NOT NULL,
    format_id BIGINT NOT NULL REFERENCES match_format(id),
    bat_value REAL,
    bowl_value REAL,
    n_samples INTEGER,
    window_spec TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(player_id, opposition_id, as_of_date, format_id)
);
CREATE INDEX IF NOT EXISTS idx_player_vs_opp_asof_player_opp_date_format
    ON player_vs_opposition_asof(player_id, opposition_id, as_of_date, format_id);

-- Player at venue snapshots as of a cutoff date (per-format)
CREATE TABLE IF NOT EXISTS player_at_venue_asof (
    id BIGSERIAL PRIMARY KEY,
    player_id BIGINT NOT NULL REFERENCES player(id),
    venue_id BIGINT NOT NULL REFERENCES venue(id),
    as_of_date DATE NOT NULL,
    format_id BIGINT NOT NULL REFERENCES match_format(id),
    bat_value REAL,
    bowl_value REAL,
    n_samples INTEGER,
    window_spec TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(player_id, venue_id, as_of_date, format_id)
);
CREATE INDEX IF NOT EXISTS idx_player_at_venue_asof_player_venue_date_format
    ON player_at_venue_asof(player_id, venue_id, as_of_date, format_id);
