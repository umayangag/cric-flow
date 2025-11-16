-- 0018_batting_transition_features.sql
-- Create table for batting transition features with latest-as-of index.

BEGIN;

CREATE TABLE IF NOT EXISTS batting_transition_features (
  as_of_date     DATE NOT NULL,
  format_id      SMALLINT NOT NULL,
  scope          VARCHAR(16) NOT NULL DEFAULT 'overall',
  scope_id       BIGINT NOT NULL DEFAULT 0,
  prev_batter_id BIGINT NOT NULL,
  batter_id      BIGINT NOT NULL,
  phase          VARCHAR(16) NOT NULL,
  balls          INTEGER NOT NULL,
  runs           INTEGER NOT NULL,
  dismissals     INTEGER NOT NULL,
  fours          INTEGER NOT NULL,
  sixes          INTEGER NOT NULL,
  strike_rate    REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN runs::float*100/balls ELSE 0 END) STORED,
  out_rate       REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN dismissals::float/balls ELSE 0 END) STORED,
  PRIMARY KEY (as_of_date, format_id, scope, scope_id, prev_batter_id, batter_id, phase)
);

CREATE INDEX IF NOT EXISTS idx_bat_trans_latest
  ON batting_transition_features (batter_id, format_id, as_of_date DESC)
  INCLUDE (prev_batter_id, phase, balls, runs, strike_rate)
  WHERE scope='overall' AND scope_id = 0;

COMMIT;
