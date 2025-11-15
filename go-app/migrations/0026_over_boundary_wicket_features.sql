-- 0026_over_boundary_wicket_features.sql
-- Over position features: ball1 and ball6 boundary/wicket incidence by bowler and phase

BEGIN;

CREATE TABLE IF NOT EXISTS over_boundary_wicket_features (
    as_of_date DATE NOT NULL,
    format_id  INT  NOT NULL,
    scope      TEXT NOT NULL DEFAULT 'overall',
    scope_id   INT,
    scope_id0  INT  GENERATED ALWAYS AS (COALESCE(scope_id, 0)) STORED,
    player_id  BIGINT NOT NULL,
    phase      TEXT NOT NULL,
    position   INT  NOT NULL, -- 1 or 6

    balls          INT    NOT NULL DEFAULT 0,
    boundaries     INT    NOT NULL DEFAULT 0,
    wickets        INT    NOT NULL DEFAULT 0,

    boundary_rate  DOUBLE PRECISION NOT NULL DEFAULT 0,
    wicket_rate    DOUBLE PRECISION NOT NULL DEFAULT 0,

    PRIMARY KEY (as_of_date, format_id, scope, scope_id0, player_id, phase, position)
);

CREATE INDEX IF NOT EXISTS idx_overpos_latest
  ON over_boundary_wicket_features (player_id, format_id, scope, scope_id0, phase, as_of_date DESC);

COMMIT;
