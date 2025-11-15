-- 0020_player_window_features.sql
-- Player rolling window features (batting and bowling) with latest-as-of snapshotting.
-- Primary key includes scope and scope_id0 (generated from COALESCE(scope_id,0)).

BEGIN;

CREATE TABLE IF NOT EXISTS player_window_features (
  as_of_date   DATE NOT NULL,
  format_id    SMALLINT NOT NULL,
  scope        VARCHAR(32) NOT NULL,
  scope_id     BIGINT,
  scope_id0    BIGINT GENERATED ALWAYS AS (COALESCE(scope_id,0)) STORED,
  player_id    BIGINT NOT NULL,
  role         VARCHAR(8) NOT NULL, -- 'bat' | 'bowl'
  phase        VARCHAR(16) NOT NULL, -- powerplay|middle|death|all
  horizon      INTEGER NOT NULL,
  -- Common counters
  balls        INTEGER NOT NULL,
  runs         INTEGER NOT NULL,
  dots         INTEGER NOT NULL DEFAULT 0,
  -- Batting-specific
  fours        INTEGER,
  sixes        INTEGER,
  dismissals   INTEGER,
  sr           REAL,
  boundary_rate REAL,
  dot_rate      REAL,
  dismissal_hazard REAL,
  -- Bowling-specific
  wickets               INTEGER,
  dot_balls             INTEGER,
  boundaries_conceded   INTEGER,
  wide_nb               INTEGER,
  econ                  REAL,
  wicket_rate           REAL,
  PRIMARY KEY (as_of_date, format_id, scope, scope_id0, player_id, role, phase, horizon)
);

-- Latest-as-of (overall) index for quick lookups
CREATE INDEX IF NOT EXISTS idx_player_window_features_latest
  ON player_window_features (player_id, role, format_id, as_of_date DESC)
  INCLUDE (phase, horizon, balls, runs, dots, fours, sixes, dismissals, sr, boundary_rate, dot_rate, dismissal_hazard,
           wickets, dot_balls, boundaries_conceded, wide_nb, econ, wicket_rate)
  WHERE scope='overall' AND scope_id IS NULL;

COMMIT;
