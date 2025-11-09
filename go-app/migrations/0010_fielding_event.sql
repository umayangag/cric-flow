-- Fielding events table: normalized store of fielding-related wicket events
-- Postgres dialect

CREATE TABLE IF NOT EXISTS fielding_event (
  id BIGSERIAL PRIMARY KEY,
  match_id BIGINT NOT NULL,
  innings SMALLINT NOT NULL,
  over SMALLINT NOT NULL,
  ball SMALLINT NOT NULL,
  batter_out_id BIGINT NULL REFERENCES player(id),
  fielder_id BIGINT NULL REFERENCES player(id),
  bowler_id BIGINT NULL REFERENCES player(id),
  kind TEXT NOT NULL CHECK (kind IN ('caught','run_out','stumped','other')),
  assist_role TEXT NOT NULL DEFAULT '', -- store empty string instead of NULL for idempotency key
  is_direct_hit BOOLEAN NOT NULL DEFAULT FALSE,
  notes TEXT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Uniqueness to ensure idempotency across re-ingests
CREATE UNIQUE INDEX IF NOT EXISTS uq_fielding_event_natural
ON fielding_event(match_id, innings, over, ball, fielder_id, kind, assist_role);

CREATE INDEX IF NOT EXISTS idx_fielding_event_match ON fielding_event(match_id);
CREATE INDEX IF NOT EXISTS idx_fielding_event_player ON fielding_event(fielder_id);
