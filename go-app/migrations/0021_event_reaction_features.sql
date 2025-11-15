-- 0021_event_reaction_features.sql
-- Create table for event-based immediate reaction features with latest-as-of index.

BEGIN;

CREATE TABLE IF NOT EXISTS event_reaction_features (
  as_of_date     DATE NOT NULL,
  format_id      SMALLINT NOT NULL,
  scope          VARCHAR(16) NOT NULL DEFAULT 'overall',
  scope_id       BIGINT NOT NULL DEFAULT 0,
  player_id      BIGINT NOT NULL,
  role           VARCHAR(8) NOT NULL CHECK (role IN ('bat','bowl')),
  prev_event     VARCHAR(16) NOT NULL,
  phase          VARCHAR(16) NOT NULL,
  -- shared basic counts
  balls          INTEGER NOT NULL,
  runs           INTEGER NOT NULL,
  -- batting-oriented
  dismissals     INTEGER NOT NULL,
  boundaries     INTEGER NOT NULL,
  -- bowling-oriented
  wickets        INTEGER NOT NULL,
  dot_balls      INTEGER NOT NULL,
  boundaries_conceded INTEGER NOT NULL,
  -- generated helper metrics
  sr             REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN runs::float*100/balls ELSE 0 END) STORED,
  boundary_rate  REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN boundaries::float/balls ELSE 0 END) STORED,
  dot_rate       REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN dot_balls::float/balls ELSE 0 END) STORED,
  wicket_rate    REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN wickets::float/balls ELSE 0 END) STORED,
  econ           REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN (runs::float*6)/balls ELSE 0 END) STORED,
  PRIMARY KEY (as_of_date, format_id, scope, scope_id, player_id, role, prev_event, phase)
);

CREATE INDEX IF NOT EXISTS idx_event_reaction_latest
  ON event_reaction_features (player_id, role, format_id, as_of_date DESC)
  INCLUDE (prev_event, phase, balls, runs)
  WHERE scope='overall' AND scope_id = 0;

COMMIT;
