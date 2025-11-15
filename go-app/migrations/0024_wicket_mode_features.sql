-- 0024_wicket_mode_features.sql
-- Wicket mode distributions by player (bowler focus; batter optional later)

BEGIN;

CREATE TABLE IF NOT EXISTS wicket_mode_features (
    as_of_date DATE NOT NULL,
    format_id  INT  NOT NULL,
    scope      TEXT NOT NULL DEFAULT 'overall',
    scope_id   INT,
    scope_id0  INT  GENERATED ALWAYS AS (COALESCE(scope_id, 0)) STORED,
    player_id  BIGINT NOT NULL,   -- bowler id
    phase      TEXT NOT NULL,
    mode       TEXT NOT NULL,     -- bowled, lbw, caught, stumped, run_out, hit_wicket, retired, obstructing,
                                  -- or project-specific canonical names

    balls          INT    NOT NULL DEFAULT 0,
    wickets        INT    NOT NULL DEFAULT 0,

    -- convenience rate: wickets per 100 balls for this mode (stored)
    wickets_per_100 DOUBLE PRECISION NOT NULL DEFAULT 0,

    PRIMARY KEY (as_of_date, format_id, scope, scope_id0, player_id, phase, mode)
);

CREATE INDEX IF NOT EXISTS idx_wicket_mode_latest
  ON wicket_mode_features (player_id, format_id, scope, scope_id0, phase, as_of_date DESC);

COMMIT;
