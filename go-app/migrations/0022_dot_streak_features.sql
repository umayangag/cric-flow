-- 0022_dot_streak_features.sql
-- Create table for next-ball outcomes after k consecutive dots with latest-as-of index.

BEGIN;

CREATE TABLE IF NOT EXISTS dot_streak_features (
  as_of_date     DATE NOT NULL,
  format_id      SMALLINT NOT NULL,
  scope          VARCHAR(16) NOT NULL DEFAULT 'overall',
  scope_id       BIGINT NOT NULL DEFAULT 0,
  player_id      BIGINT NOT NULL,
  role           VARCHAR(8) NOT NULL CHECK (role IN ('bat','bowl')),
  k              SMALLINT NOT NULL,
  phase          VARCHAR(16) NOT NULL,
  -- totals and categoricals for the next ball after a k-dot streak
  balls              INTEGER NOT NULL,
  runs_next_total    INTEGER NOT NULL,
  next_boundary      INTEGER NOT NULL,
  next_single        INTEGER NOT NULL,
  next_wicket        INTEGER NOT NULL,
  next_extra         INTEGER NOT NULL,
  next_dot           INTEGER NOT NULL,
  -- generated probabilities and averages (safe zeros)
  avg_runs_next_ball REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN runs_next_total::float/balls ELSE 0 END) STORED,
  p_boundary         REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN next_boundary::float/balls ELSE 0 END) STORED,
  p_single           REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN next_single::float/balls ELSE 0 END) STORED,
  p_wicket           REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN next_wicket::float/balls ELSE 0 END) STORED,
  p_extra            REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN next_extra::float/balls ELSE 0 END) STORED,
  p_dot              REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN next_dot::float/balls ELSE 0 END) STORED,
  PRIMARY KEY (as_of_date, format_id, scope, scope_id, player_id, role, k, phase)
);

CREATE INDEX IF NOT EXISTS idx_dot_streak_latest
  ON dot_streak_features (player_id, role, format_id, as_of_date DESC)
  INCLUDE (k, phase, balls, runs_next_total)
  WHERE scope='overall' AND scope_id = 0;

COMMIT;
