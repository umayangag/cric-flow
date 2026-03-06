-- Migration: raw windowed statistics snapshots (multi-scale stats for ML to learn form/consistency)
-- Replaces formula-derived form/consistency with parameter-free summary statistics per scope.

CREATE TABLE IF NOT EXISTS feature_raw_stats_snapshots (
    id BIGSERIAL PRIMARY KEY,
    player_id   BIGINT      NOT NULL REFERENCES player(id),
    as_of_date  DATE        NOT NULL,
    format_id   BIGINT      NOT NULL REFERENCES match_format(id),
    scope       TEXT        NOT NULL CHECK (scope IN ('overall','venue','opposition')),
    scope_id    BIGINT      NULL,

    -- Batting raw stats (18)
    batting_mean_w3 REAL NOT NULL DEFAULT 0,
    batting_mean_w5 REAL NOT NULL DEFAULT 0,
    batting_mean_w10 REAL NOT NULL DEFAULT 0,
    batting_mean_w20 REAL NOT NULL DEFAULT 0,
    batting_std_w5 REAL NOT NULL DEFAULT 0,
    batting_std_w10 REAL NOT NULL DEFAULT 0,
    batting_max_w10 REAL NOT NULL DEFAULT 0,
    batting_min_w10 REAL NOT NULL DEFAULT 0,
    batting_median_w10 REAL NOT NULL DEFAULT 0,
    batting_last_1 REAL NOT NULL DEFAULT 0,
    batting_last_2 REAL NOT NULL DEFAULT 0,
    batting_last_3 REAL NOT NULL DEFAULT 0,
    batting_career_mean REAL NOT NULL DEFAULT 0,
    batting_career_count INT NOT NULL DEFAULT 0,
    batting_pct_zero_w10 REAL NOT NULL DEFAULT 0,
    batting_trend_w5 REAL NOT NULL DEFAULT 0,
    batting_days_since_last REAL NOT NULL DEFAULT 0,
    batting_innings_in_last_90d INT NOT NULL DEFAULT 0,

    -- Bowling raw stats (18)
    bowling_mean_w3 REAL NOT NULL DEFAULT 0,
    bowling_mean_w5 REAL NOT NULL DEFAULT 0,
    bowling_mean_w10 REAL NOT NULL DEFAULT 0,
    bowling_mean_w20 REAL NOT NULL DEFAULT 0,
    bowling_std_w5 REAL NOT NULL DEFAULT 0,
    bowling_std_w10 REAL NOT NULL DEFAULT 0,
    bowling_max_w10 REAL NOT NULL DEFAULT 0,
    bowling_min_w10 REAL NOT NULL DEFAULT 0,
    bowling_median_w10 REAL NOT NULL DEFAULT 0,
    bowling_last_1 REAL NOT NULL DEFAULT 0,
    bowling_last_2 REAL NOT NULL DEFAULT 0,
    bowling_last_3 REAL NOT NULL DEFAULT 0,
    bowling_career_mean REAL NOT NULL DEFAULT 0,
    bowling_career_count INT NOT NULL DEFAULT 0,
    bowling_pct_zero_w10 REAL NOT NULL DEFAULT 0,
    bowling_trend_w5 REAL NOT NULL DEFAULT 0,
    bowling_days_since_last REAL NOT NULL DEFAULT 0,
    bowling_innings_in_last_90d INT NOT NULL DEFAULT 0,

    source_version TEXT NOT NULL DEFAULT 'v1',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(player_id, as_of_date, format_id, scope, scope_id)
);

CREATE INDEX IF NOT EXISTS idx_feature_raw_stats_player_fmt_date
    ON feature_raw_stats_snapshots(player_id, format_id, as_of_date);
CREATE INDEX IF NOT EXISTS idx_feature_raw_stats_scope
    ON feature_raw_stats_snapshots(scope, scope_id, player_id, as_of_date);
