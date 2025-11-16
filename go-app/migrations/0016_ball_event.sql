-- 0016_ball_event.sql
-- Create ball_event table and supporting indexes for sequence computations.
-- Notes:
-- - ball_seq counts only legal deliveries; illegal balls (wides/no-balls) set is_legal=false
--   and share the prior legal ball_seq for ordering context. Use (over, ball) for exact order.

BEGIN;

CREATE TABLE IF NOT EXISTS ball_event (
  match_id       BIGINT NOT NULL,
  innings        SMALLINT NOT NULL,
  over           SMALLINT NOT NULL,
  ball           SMALLINT NOT NULL,
  ball_seq       INTEGER NOT NULL,
  is_legal       BOOLEAN NOT NULL,
  phase          VARCHAR(16) NOT NULL,
  striker_id     BIGINT,
  non_striker_id BIGINT,
  bowler_id      BIGINT,
  runs_batter    SMALLINT NOT NULL DEFAULT 0,
  runs_extras    SMALLINT NOT NULL DEFAULT 0,
  runs_total     SMALLINT NOT NULL DEFAULT 0,
  extras_kind    VARCHAR(16),
  wicket_kind    VARCHAR(24),
  player_out_id  BIGINT,
  fielder_ids    BIGINT[],
  PRIMARY KEY (match_id, innings, over, ball)
);

-- Indexes to support common sequence scans
CREATE INDEX IF NOT EXISTS idx_ball_event_match_innings_seq
  ON ball_event (match_id, innings, ball_seq);

CREATE INDEX IF NOT EXISTS idx_ball_event_bowler_seq
  ON ball_event (bowler_id, match_id, innings, ball_seq)
  WHERE bowler_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ball_event_striker_seq
  ON ball_event (striker_id, match_id, innings, ball_seq)
  WHERE striker_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ball_event_phase
  ON ball_event (phase);

COMMIT;