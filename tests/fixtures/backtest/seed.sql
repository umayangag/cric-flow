-- Tiny deterministic fixtures for backtest E2E smoke
-- This script is intentionally defensive: it creates minimal tables only if they
-- do not already exist (for dev environments that don’t have them yet), and
-- inserts a single played T20 match IND vs AUS with a handful of player rows
-- and batting/bowling actuals. It is safe to run multiple times.

BEGIN;

-- Minimal tables that some environments may be missing (no-ops if they exist)
CREATE TABLE IF NOT EXISTS team (
    id   BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS team_match (
    match_id BIGINT NOT NULL,
    team_id  BIGINT NOT NULL REFERENCES team(id),
    result   VARCHAR(16),
    UNIQUE(match_id, team_id)
);

CREATE TABLE IF NOT EXISTS match_format (
    id   BIGSERIAL PRIMARY KEY,
    code VARCHAR(16) NOT NULL UNIQUE
);

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
INSERT INTO match_format(code) VALUES ('T20') ON CONFLICT (code) DO NOTHING;
INSERT INTO season(season_name) VALUES ('2024') ON CONFLICT (season_name) DO NOTHING;
INSERT INTO venue(venue_name) VALUES ('Wankhede Stadium') ON CONFLICT (venue_name) DO NOTHING;

INSERT INTO team(name) VALUES ('IND') ON CONFLICT (name) DO NOTHING;
INSERT INTO team(name) VALUES ('AUS') ON CONFLICT (name) DO NOTHING;

-- Resolve IDs
WITH s AS (
  SELECT id AS season_id FROM season WHERE season_name='2024'
), v AS (
  SELECT id AS venue_id FROM venue WHERE venue_name='Wankhede Stadium'
), f AS (
  SELECT id AS format_id FROM match_format WHERE code='T20'
)
-- Insert a deterministic played match with totals
INSERT INTO match_details(id, score, wickets, overs, balls, rpo, target, inning, result, opposition_id, date, match_id,
                          batting_session, bowling_session, venue_id, extras, toss, season_id, match_number, format_id)
SELECT
  999001,
  150,         -- score (runs)
  7,           -- wickets
  20.0,        -- overs (optional)
  120,         -- balls (optional)
  7.5,         -- rpo (optional)
  0,           -- target (not used in backtest metrics)
  1,
  1,           -- result (arbitrary)
  NULL,        -- opposition_id unused in backtest
  NOW() - INTERVAL '30 days',
  9000111,     -- match_id
  'A', 'B',
  (SELECT venue_id FROM v),
  10,          -- extras
  'IND',       -- toss (arbitrary)
  (SELECT season_id FROM s),
  1,
  (SELECT format_id FROM f)
WHERE NOT EXISTS (SELECT 1 FROM match_details WHERE match_id = 9000111);

-- Link teams and mark winner (IND)
WITH ind AS (SELECT id AS team_id FROM team WHERE name='IND'),
     aus AS (SELECT id AS team_id FROM team WHERE name='AUS')
INSERT INTO team_match(match_id, team_id, result)
SELECT 9000111, (SELECT team_id FROM ind), 'W'
ON CONFLICT DO NOTHING;

WITH aus AS (SELECT id AS team_id FROM team WHERE name='AUS')
INSERT INTO team_match(match_id, team_id, result)
SELECT 9000111, (SELECT team_id FROM aus), 'L'
ON CONFLICT DO NOTHING;

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
