-- Tiny deterministic fixtures for backtest E2E smoke
-- This script is intentionally defensive: it creates minimal tables only if they
-- do not already exist (for dev environments that don’t have them yet), and
-- inserts a single played T20 match IND vs AUS with a handful of player rows
-- and batting/bowling actuals. It is safe to run multiple times.
-- Updated to match real database structure using opposition table.

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

-- If any rows have NULL name, set it to the code value for safety
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

-- Ensure venue has a canonical column name used by the app (name)
DO $$
BEGIN
  IF NOT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_name='venue' AND column_name='name'
  ) THEN
    ALTER TABLE venue ADD COLUMN name VARCHAR(200);
  END IF;
END$$;

-- Some older schemas may not have match_details.format_id; add if missing (best-effort)
DO $$
BEGIN
  IF NOT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_name='match_details' AND column_name='format_id'
  ) THEN
    BEGIN
      ALTER TABLE match_details ADD COLUMN format_id BIGINT;
    EXCEPTION WHEN duplicate_column THEN
      -- another process added it, ignore
      NULL;
    END;
  END IF;
END$$;

-- Seed lookup rows
INSERT INTO match_format(code, name)
VALUES ('T20', 'T20 (All)')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name;
-- Insert season/venue using whichever column layout exists, then keep 'name' in sync
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

DO $$
BEGIN
  IF EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_name='venue' AND column_name='venue_name'
  ) THEN
    INSERT INTO venue(venue_name) VALUES ('Wankhede Stadium') ON CONFLICT (venue_name) DO NOTHING;
  ELSE
    INSERT INTO venue(name) VALUES ('Wankhede Stadium') ON CONFLICT (name) DO NOTHING;
  END IF;
END$$;

-- Re-sync canonical 'name' columns after inserts
DO $$
BEGIN
  IF EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_name='season' AND column_name='season_name'
  ) THEN
    UPDATE season SET name = COALESCE(name, season_name);
  END IF;
END$$;

DO $$
BEGIN
  IF EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_name='venue' AND column_name='venue_name'
  ) THEN
    UPDATE venue SET name = COALESCE(name, venue_name);
  END IF;
END$$;

-- Insert opposition teams
INSERT INTO opposition(opposition_name) VALUES ('IND') ON CONFLICT (opposition_name) DO NOTHING;
INSERT INTO opposition(opposition_name) VALUES ('AUS') ON CONFLICT (opposition_name) DO NOTHING;

-- Resolve IDs
WITH s AS (
  SELECT id AS season_id FROM season WHERE COALESCE(name, season_name)='2024'
), v AS (
  SELECT id AS venue_id FROM venue WHERE COALESCE(name, venue_name)='Wankhede Stadium'
), f AS (
  SELECT id AS format_id FROM match_format WHERE code='T20'
), ind AS (
  SELECT id AS ind_id FROM opposition WHERE opposition_name='IND'
), aus AS (
  SELECT id AS aus_id FROM opposition WHERE opposition_name='AUS'
)
-- Insert match_details rows: one row per inning, each with different opposition_id
-- Row 1: IND batting (opposition_id = IND)
INSERT INTO match_details(id, score, wickets, overs, balls, rpo, target, inning, result, opposition_id, match_date, match_id,
                          batting_session, bowling_session, venue_id, extras, toss, season_id, match_number, format_id)
SELECT
  999001,
  150,         -- score (runs)
  7,           -- wickets
  20.0,        -- overs
  120,         -- balls
  7.5,         -- rpo
  0,           -- target
  1,           -- inning 1
  (SELECT ind_id FROM ind),  -- result points to winner (IND)
  (SELECT ind_id FROM ind),  -- opposition_id = IND (batting team)
  NOW() - INTERVAL '30 days',
  9000111,     -- match_id
  'A', 'B',
  (SELECT venue_id FROM v),
  10,          -- extras
  'IND',       -- toss
  (SELECT season_id FROM s),
  1,
  (SELECT format_id FROM f)
WHERE NOT EXISTS (SELECT 1 FROM match_details WHERE match_id = 9000111 AND inning = 1);

-- Row 2: AUS batting (opposition_id = AUS)
WITH s AS (
  SELECT id AS season_id FROM season WHERE COALESCE(name, season_name)='2024'
), v AS (
  SELECT id AS venue_id FROM venue WHERE COALESCE(name, venue_name)='Wankhede Stadium'
), f AS (
  SELECT id AS format_id FROM match_format WHERE code='T20'
), ind AS (
  SELECT id AS ind_id FROM opposition WHERE opposition_name='IND'
), aus AS (
  SELECT id AS aus_id FROM opposition WHERE opposition_name='AUS'
)
INSERT INTO match_details(id, score, wickets, overs, balls, rpo, target, inning, result, opposition_id, match_date, match_id,
                          batting_session, bowling_session, venue_id, extras, toss, season_id, match_number, format_id)
SELECT
  999002,
  140,         -- score (runs) - AUS scored less
  9,           -- wickets
  20.0,        -- overs
  120,         -- balls
  7.0,         -- rpo
  151,         -- target (IND's score + 1)
  2,           -- inning 2
  (SELECT ind_id FROM ind),  -- result points to winner (IND)
  (SELECT aus_id FROM aus),  -- opposition_id = AUS (batting team)
  NOW() - INTERVAL '30 days',
  9000111,     -- match_id (same match)
  'C', 'D',
  (SELECT venue_id FROM v),
  8,           -- extras
  'IND',       -- toss
  (SELECT season_id FROM s),
  1,
  (SELECT format_id FROM f)
WHERE NOT EXISTS (SELECT 1 FROM match_details WHERE match_id = 9000111 AND inning = 2);

-- Players
INSERT INTO player(player_name, is_wicket_keeper, is_retired)
VALUES ('IND_Player_1', 0, 0),
       ('IND_Player_2', 0, 0),
       ('AUS_Player_1', 0, 0),
       ('AUS_Player_2', 0, 0)
ON CONFLICT (player_name) DO NOTHING;

-- Resolve player IDs for deterministic mapping
WITH p AS (
  SELECT player_name, id AS pid FROM player WHERE player_name IN ('IND_Player_1','IND_Player_2','AUS_Player_1','AUS_Player_2')
)
-- Batting actuals
INSERT INTO batting_data(match_id, player_id, description, runs, balls, minutes, fours, sixes, strike_rate, batting_position)
SELECT 9000111, (SELECT pid FROM p WHERE player_name='IND_Player_1'), 'bat', 30, 20, 30, 4, 1, 150.0, 1
UNION ALL
SELECT 9000111, (SELECT pid FROM p WHERE player_name='IND_Player_2'), 'bat', 10, 12, 15, 1, 0, 83.3, 2
UNION ALL
SELECT 9000111, (SELECT pid FROM p WHERE player_name='AUS_Player_1'), 'bat', 5, 10, 12, 0, 0, 50.0, 1
UNION ALL
SELECT 9000111, (SELECT pid FROM p WHERE player_name='AUS_Player_2'), 'bat', 0, 2, 3, 0, 0, 0.0, 2
ON CONFLICT (match_id, player_id) DO NOTHING;

-- Bowling actuals
WITH p AS (
  SELECT player_name, id AS pid FROM player WHERE player_name IN ('IND_Player_1','IND_Player_2','AUS_Player_1','AUS_Player_2')
)
INSERT INTO bowling_data(match_id, player_id, overs, balls, maidens, runs, wickets, dots, fours, sixes, econ, wides, no_balls)
SELECT 9000111, (SELECT pid FROM p WHERE player_name='IND_Player_1'), 4.0, 24, 0, 30, 1, 10, 2, 1, 7.5, 1, 0
UNION ALL
SELECT 9000111, (SELECT pid FROM p WHERE player_name='AUS_Player_1'), 4.0, 24, 0, 28, 2, 12, 3, 0, 7.0, 0, 0
ON CONFLICT (match_id, player_id) DO NOTHING;

COMMIT;
