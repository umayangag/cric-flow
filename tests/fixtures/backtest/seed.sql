-- Tiny deterministic fixtures for backtest E2E smoke
-- This script is intentionally defensive: it creates minimal tables only if they
-- do not already exist (for dev environments that don't have them yet), and
-- inserts a single played T20 match IND vs AUS with a handful of player rows
-- and batting/bowling actuals. It is safe to run multiple times.
-- Uses match + match_inning schema (post-0090).

BEGIN;

-- Minimal tables that some environments may be missing (no-ops if they exist)
CREATE TABLE IF NOT EXISTS opposition (
    id BIGSERIAL PRIMARY KEY,
    opposition_name VARCHAR(100) NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS match_format (
    id   BIGSERIAL PRIMARY KEY,
    code VARCHAR(16) NOT NULL UNIQUE,
    name VARCHAR(100)
);

-- Ensure legacy schemas get the name column; then backfill and conform
DO $$
BEGIN
  IF NOT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_name='match_format' AND column_name='name'
  ) THEN
    ALTER TABLE match_format ADD COLUMN name VARCHAR(100);
  END IF;
END$$;

UPDATE match_format SET name = code WHERE name IS NULL;

-- Ensure season has a canonical column name used by the app (name)
DO $$
BEGIN
  IF NOT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_name='season' AND column_name='name'
  ) THEN
    ALTER TABLE season ADD COLUMN name VARCHAR(100);
  END IF;
END$$;

-- Ensure venue has a canonical column name used by the app
DO $$
BEGIN
  IF NOT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_name='venue' AND column_name='venue_name'
  ) THEN
    ALTER TABLE venue ADD COLUMN venue_name VARCHAR(200);
  END IF;
  IF NOT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_name='venue' AND column_name='name'
  ) THEN
    ALTER TABLE venue ADD COLUMN name VARCHAR(200);
  END IF;
END$$;

-- Seed lookup rows
INSERT INTO match_format(code, name)
VALUES ('T20', 'T20 (All)')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name;

DO $$
BEGIN
  IF EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_name='season' AND column_name='season_name'
  ) THEN
    INSERT INTO season(season_name) VALUES ('2024') ON CONFLICT (season_name) DO NOTHING;
  ELSE
    INSERT INTO season(name) VALUES ('2024') ON CONFLICT (name) DO NOTHING;
  END IF;
END$$;

INSERT INTO venue(venue_name) VALUES ('Wankhede Stadium') ON CONFLICT (venue_name) DO NOTHING;

DO $$
BEGIN
  IF EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_name='season' AND column_name='season_name'
  ) THEN
    UPDATE season SET name = COALESCE(name, season_name);
  END IF;
END$$;

-- Insert opposition teams
INSERT INTO opposition(opposition_name) VALUES ('IND') ON CONFLICT (opposition_name) DO NOTHING;
INSERT INTO opposition(opposition_name) VALUES ('AUS') ON CONFLICT (opposition_name) DO NOTHING;

-- Resolve IDs and insert match + match_inning (schema post-0090)
-- match table must exist (created by migration 0090)
WITH s AS (
  SELECT id AS season_id FROM season WHERE COALESCE(name, season_name)='2024' ORDER BY id LIMIT 1
), v AS (
  SELECT id AS venue_id FROM venue WHERE COALESCE(name, venue_name)='Wankhede Stadium' ORDER BY id LIMIT 1
), f AS (
  SELECT id AS format_id FROM match_format WHERE code='T20' ORDER BY id LIMIT 1
), ind AS (
  SELECT id AS ind_id FROM opposition WHERE opposition_name='IND' ORDER BY id LIMIT 1
)
INSERT INTO match (
  match_id, format_id, match_date, original_match_type, venue_id, season_id,
  toss_winner_opposition_id, toss_decision, outcome_winner_opposition_id, match_number, balls_per_over
)
SELECT
  9000111,
  (SELECT format_id FROM f),
  (NOW() - INTERVAL '30 days')::date,
  'T20',
  (SELECT venue_id FROM v),
  (SELECT season_id FROM s),
  (SELECT ind_id FROM ind),
  'bat',
  (SELECT ind_id FROM ind),
  1,
  6
WHERE NOT EXISTS (SELECT 1 FROM match WHERE match_id = 9000111);

-- Inning 1: IND batting, AUS bowling. Runs 150, wickets 7
WITH ind AS (SELECT id AS ind_id FROM opposition WHERE opposition_name='IND' ORDER BY id LIMIT 1),
     aus AS (SELECT id AS aus_id FROM opposition WHERE opposition_name='AUS' ORDER BY id LIMIT 1)
INSERT INTO match_inning (
  match_id, inning_number, batting_team_opposition_id, bowling_team_opposition_id,
  runs_scored, wickets_lost, overs_bowled, balls_bowled, run_rate, target_runs, extras, winner_opposition_id
)
SELECT 9000111, 1, (SELECT ind_id FROM ind), (SELECT aus_id FROM aus),
  150, 7, 20.0, 120, 7.5, 0, 10, (SELECT ind_id FROM ind)
WHERE NOT EXISTS (SELECT 1 FROM match_inning WHERE match_id = 9000111 AND inning_number = 1);

-- Inning 2: AUS batting, IND bowling. Runs 140, target 151
WITH ind AS (SELECT id AS ind_id FROM opposition WHERE opposition_name='IND' ORDER BY id LIMIT 1),
     aus AS (SELECT id AS aus_id FROM opposition WHERE opposition_name='AUS' ORDER BY id LIMIT 1)
INSERT INTO match_inning (
  match_id, inning_number, batting_team_opposition_id, bowling_team_opposition_id,
  runs_scored, wickets_lost, overs_bowled, balls_bowled, run_rate, target_runs, extras, winner_opposition_id
)
SELECT 9000111, 2, (SELECT aus_id FROM aus), (SELECT ind_id FROM ind),
  140, 9, 20.0, 120, 7.0, 151, 8, (SELECT ind_id FROM ind)
WHERE NOT EXISTS (SELECT 1 FROM match_inning WHERE match_id = 9000111 AND inning_number = 2);

-- Players
INSERT INTO player(player_name, is_wicket_keeper, is_retired)
VALUES ('IND_Player_1', 0, 0),
       ('IND_Player_2', 0, 0),
       ('AUS_Player_1', 0, 0),
       ('AUS_Player_2', 0, 0)
ON CONFLICT (player_name) DO NOTHING;

-- Batting actuals: inning 1 = IND batsmen, inning 2 = AUS batsmen
WITH p AS (
  SELECT player_name, id AS pid FROM player WHERE player_name IN ('IND_Player_1','IND_Player_2','AUS_Player_1','AUS_Player_2')
)
INSERT INTO batting_data(match_id, inning_number, player_id, description, runs, balls, minutes, fours, sixes, strike_rate, batting_position)
SELECT 9000111, 1, (SELECT pid FROM p WHERE player_name='IND_Player_1'), 'bat', 30, 20, 30, 4, 1, 150.0, 1
UNION ALL
SELECT 9000111, 1, (SELECT pid FROM p WHERE player_name='IND_Player_2'), 'bat', 10, 12, 15, 1, 0, 83.3, 2
UNION ALL
SELECT 9000111, 2, (SELECT pid FROM p WHERE player_name='AUS_Player_1'), 'bat', 5, 10, 12, 0, 0, 50.0, 1
UNION ALL
SELECT 9000111, 2, (SELECT pid FROM p WHERE player_name='AUS_Player_2'), 'bat', 0, 2, 3, 0, 0, 0.0, 2
ON CONFLICT (match_id, inning_number, player_id) DO NOTHING;

-- Bowling actuals: inning 1 = AUS bowlers, inning 2 = IND bowlers
WITH p AS (
  SELECT player_name, id AS pid FROM player WHERE player_name IN ('IND_Player_1','IND_Player_2','AUS_Player_1','AUS_Player_2')
)
INSERT INTO bowling_data(match_id, inning_number, player_id, overs, balls, maidens, runs, wickets, dots, fours, sixes, econ, wides, no_balls)
SELECT 9000111, 1, (SELECT pid FROM p WHERE player_name='AUS_Player_1'), 4.0, 24, 0, 28, 2, 12, 3, 0, 7.0, 0, 0
UNION ALL
SELECT 9000111, 2, (SELECT pid FROM p WHERE player_name='IND_Player_1'), 4.0, 24, 0, 30, 1, 10, 2, 1, 7.5, 1, 0
ON CONFLICT (match_id, inning_number, player_id) DO NOTHING;

COMMIT;
