-- 0019_bowling_sequence_features.sql
-- Create bowling_sequence_features table to capture over-to-over bowler pair effects.
-- Primary key uses latest-as-of snapshot pattern keyed by (as_of_date, format_id, scope, COALESCE(scope_id,0), prev_bowler_id, bowler_id, phase).

BEGIN;

CREATE TABLE IF NOT EXISTS bowling_sequence_features (
  as_of_date     DATE NOT NULL,
  format_id      SMALLINT NOT NULL,
  scope          VARCHAR(16) NOT NULL DEFAULT 'overall',
  scope_id       BIGINT,
  scope_id0      BIGINT GENERATED ALWAYS AS (COALESCE(scope_id,0)) STORED,
  prev_bowler_id BIGINT NOT NULL,
  bowler_id      BIGINT NOT NULL,
  phase          VARCHAR(16) NOT NULL,
  overs_pairs    INTEGER NOT NULL,
  balls          INTEGER NOT NULL,
  runs           INTEGER NOT NULL,
  wickets        INTEGER NOT NULL,
  dot_balls      INTEGER NOT NULL,
  PRIMARY KEY (as_of_date, format_id, scope, scope_id0, prev_bowler_id, bowler_id, phase)
);

-- Latest-as-of index for efficient lookups of most recent aggregates (overall scope only)
CREATE INDEX IF NOT EXISTS idx_bowl_seq_latest
  ON bowling_sequence_features (bowler_id, format_id, as_of_date DESC)
  INCLUDE (prev_bowler_id, phase, overs_pairs, balls, runs, wickets, dot_balls)
  WHERE scope='overall' AND scope_id IS NULL;

COMMIT;
