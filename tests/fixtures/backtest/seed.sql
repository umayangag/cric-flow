-- Tiny deterministic fixtures for backtest E2E smoke.
--
-- Inserts a single played T20 match (IND vs AUS) with a handful of player rows and
-- batting/bowling actuals. Safe to run multiple times: every insert is guarded by
-- ON CONFLICT or NOT EXISTS.
--
-- Assumes the schema from go-app/migrations/0001_baseline.sql. It does not create or
-- alter tables: an earlier version carried defensive CREATE TABLE IF NOT EXISTS and
-- ALTER TABLE ... ADD COLUMN blocks to cope with the pre-0090 migration chain, and
-- two of them silently added season.name and venue.name -- columns no migration
-- creates -- so the smoke test ran against a schema that did not match production.

BEGIN;

-- Lookup rows. match_format already carries TEST/ODI/T20/T20I from the baseline seed;
-- this is a no-op there and only matters if the row was removed by hand.
INSERT INTO match_format(code, name)
VALUES ('T20', 'T20 (All)')
ON CONFLICT (code) DO NOTHING;

INSERT INTO season(season_name) VALUES ('2024') ON CONFLICT (season_name) DO NOTHING;

INSERT INTO venue(venue_name) VALUES ('Wankhede Stadium') ON CONFLICT (venue_name) DO NOTHING;

-- A team is (name, gender) since migration 0004_identity.sql: 130 of the 394 names in the
-- real dataset belong to both a men's and a women's side.
INSERT INTO opposition(opposition_name, gender) VALUES ('IND', 'male')
ON CONFLICT (opposition_name, gender) DO NOTHING;
INSERT INTO opposition(opposition_name, gender) VALUES ('AUS', 'male')
ON CONFLICT (opposition_name, gender) DO NOTHING;

-- Match header
WITH s AS (
  SELECT id AS season_id FROM season WHERE season_name = '2024' ORDER BY id LIMIT 1
), v AS (
  SELECT id AS venue_id FROM venue WHERE venue_name = 'Wankhede Stadium' ORDER BY id LIMIT 1
), f AS (
  SELECT id AS format_id FROM match_format WHERE code = 'T20' ORDER BY id LIMIT 1
), ind AS (
  SELECT id AS ind_id FROM opposition WHERE opposition_name = 'IND' ORDER BY id LIMIT 1
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

-- Players. external_id is the identity (the Cricsheet person identifier); player_name is a
-- display attribute and is no longer unique, so the conflict target is the identifier.
INSERT INTO player(external_id, player_name, name_as_of, is_wicket_keeper, is_retired)
VALUES ('e2e00001', 'IND_Player_1', '2024-05-01', 0, 0),
       ('e2e00002', 'IND_Player_2', '2024-05-01', 0, 0),
       ('e2e00003', 'AUS_Player_1', '2024-05-01', 0, 0),
       ('e2e00004', 'AUS_Player_2', '2024-05-01', 0, 0)
ON CONFLICT (external_id) DO NOTHING;

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
