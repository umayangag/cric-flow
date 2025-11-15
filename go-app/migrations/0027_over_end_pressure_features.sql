-- 0027_over_end_pressure_features.sql
-- End-of-over pressure features: positions 5 and 6 (legal deliveries only)

BEGIN;

CREATE TABLE IF NOT EXISTS over_end_pressure_features (
    as_of_date DATE NOT NULL,
    format_id  INT  NOT NULL,
    scope      TEXT NOT NULL DEFAULT 'overall',
    scope_id   INT,
    scope_id0  INT  GENERATED ALWAYS AS (COALESCE(scope_id, 0)) STORED,
    player_id  BIGINT NOT NULL,
    phase      TEXT NOT NULL,
    position   INT  NOT NULL, -- 5 or 6 (legal delivery index within over)

    balls          INT    NOT NULL DEFAULT 0,
    boundaries     INT    NOT NULL DEFAULT 0,
    wickets        INT    NOT NULL DEFAULT 0,

    boundary_rate  DOUBLE PRECISION NOT NULL DEFAULT 0,
    wicket_rate    DOUBLE PRECISION NOT NULL DEFAULT 0,

    PRIMARY KEY (as_of_date, format_id, scope, scope_id0, player_id, phase, position)
);

CREATE INDEX IF NOT EXISTS idx_over_end_pressure_latest
  ON over_end_pressure_features (player_id, format_id, scope, scope_id0, phase, as_of_date DESC);

COMMIT;
