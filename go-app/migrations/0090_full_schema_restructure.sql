-- Migration 0090: Full schema restructure for Cricsheet JSON alignment.
-- Drops match_details, creates match + match_inning, adds inning to batting/bowling.
-- Data is reproducible; dependent tables are truncated.

BEGIN;

-- 1) Truncate dependent tables (preserves structure; data will be re-imported)
TRUNCATE TABLE ball_event RESTART IDENTITY CASCADE;
TRUNCATE TABLE fielding_event RESTART IDENTITY CASCADE;
TRUNCATE TABLE fielding_data RESTART IDENTITY CASCADE;
TRUNCATE TABLE batting_data RESTART IDENTITY CASCADE;
TRUNCATE TABLE bowling_data RESTART IDENTITY CASCADE;
TRUNCATE TABLE weather_data RESTART IDENTITY CASCADE;

-- 2) Drop feature/snapshot tables that depend on match/batting/bowling (if they exist)
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'feature_form_snapshots') THEN
    TRUNCATE TABLE feature_form_snapshots RESTART IDENTITY CASCADE;
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'feature_consistency_snapshots') THEN
    TRUNCATE TABLE feature_consistency_snapshots RESTART IDENTITY CASCADE;
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'feature_venue_snapshots') THEN
    TRUNCATE TABLE feature_venue_snapshots RESTART IDENTITY CASCADE;
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'feature_opposition_snapshots') THEN
    TRUNCATE TABLE feature_opposition_snapshots RESTART IDENTITY CASCADE;
  END IF;
END $$;

-- 3) Drop match_details
DROP TABLE IF EXISTS match_details CASCADE;

-- 4) Create match (central match-level data)
CREATE TABLE match (
  match_id                 BIGINT PRIMARY KEY,
  format_id                BIGINT NOT NULL REFERENCES match_format(id),
  match_date               DATE NOT NULL,
  original_match_type      VARCHAR(100) NOT NULL,
  venue_id                 BIGINT REFERENCES venue(id),
  season_id                BIGINT REFERENCES season(id),
  toss_winner_opposition_id BIGINT REFERENCES opposition(id),
  toss_decision            VARCHAR(16),
  outcome_winner_opposition_id BIGINT REFERENCES opposition(id),
  outcome_by_runs          INT,
  outcome_by_wickets       INT,
  event_name               VARCHAR(255),
  match_number             INT,
  gender                   VARCHAR(16),
  balls_per_over           SMALLINT NOT NULL DEFAULT 6,
  scheduled_overs_per_innings INT
);

CREATE INDEX idx_match_format_id ON match(format_id);
CREATE INDEX idx_match_match_date ON match(match_date);
CREATE INDEX idx_match_venue_id ON match(venue_id);
CREATE INDEX idx_match_season_id ON match(season_id);

-- 5) Create match_inning (inning-level data)
CREATE TABLE match_inning (
  match_id                      BIGINT NOT NULL REFERENCES match(match_id) ON DELETE CASCADE,
  inning_number                 SMALLINT NOT NULL,
  batting_team_opposition_id    BIGINT NOT NULL REFERENCES opposition(id),
  bowling_team_opposition_id    BIGINT NOT NULL REFERENCES opposition(id),
  runs_scored                   INT NOT NULL DEFAULT 0,
  wickets_lost                  INT NOT NULL DEFAULT 0,
  overs_bowled                  REAL NOT NULL DEFAULT 0,
  balls_bowled                  INT NOT NULL DEFAULT 0,
  run_rate                      REAL,
  target_runs                   INT,
  extras                        INT NOT NULL DEFAULT 0,
  winner_opposition_id          BIGINT REFERENCES opposition(id),
  PRIMARY KEY (match_id, inning_number)
);

CREATE INDEX idx_match_inning_match_id ON match_inning(match_id);
CREATE INDEX idx_match_inning_batting_team ON match_inning(batting_team_opposition_id);
CREATE INDEX idx_match_inning_bowling_team ON match_inning(bowling_team_opposition_id);

-- 6) Add inning_number to batting_data, update unique constraint
ALTER TABLE batting_data ADD COLUMN inning_number SMALLINT NOT NULL DEFAULT 1;
-- Drop both explicit (0002) and auto-generated (0001) old constraints
ALTER TABLE batting_data DROP CONSTRAINT IF EXISTS uq_batting_match_player;
ALTER TABLE batting_data DROP CONSTRAINT IF EXISTS batting_data_match_id_player_id_key;
ALTER TABLE batting_data DROP CONSTRAINT IF EXISTS uq_batting_match_inning_player;
ALTER TABLE batting_data ADD CONSTRAINT uq_batting_match_inning_player UNIQUE (match_id, inning_number, player_id);

CREATE INDEX IF NOT EXISTS idx_batting_data_match_inning ON batting_data(match_id, inning_number);

-- 7) Add inning_number to bowling_data, update unique constraint
ALTER TABLE bowling_data ADD COLUMN inning_number SMALLINT NOT NULL DEFAULT 1;
-- Drop both explicit (0002) and auto-generated (0001) old constraints
ALTER TABLE bowling_data DROP CONSTRAINT IF EXISTS uq_bowling_match_player;
ALTER TABLE bowling_data DROP CONSTRAINT IF EXISTS bowling_data_match_id_player_id_key;
ALTER TABLE bowling_data DROP CONSTRAINT IF EXISTS uq_bowling_match_inning_player;
ALTER TABLE bowling_data ADD CONSTRAINT uq_bowling_match_inning_player UNIQUE (match_id, inning_number, player_id);

CREATE INDEX IF NOT EXISTS idx_bowling_data_match_inning ON bowling_data(match_id, inning_number);

COMMIT;
