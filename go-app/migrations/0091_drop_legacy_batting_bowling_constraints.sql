-- Migration 0091: Drop legacy auto-generated unique constraints on batting_data and bowling_data.
-- 0001_init created UNIQUE (match_id, player_id) which PostgreSQL named batting_data_match_id_player_id_key
-- and bowling_data_match_id_player_id_key. Migration 0090 dropped uq_batting_match_player but not these
-- auto-generated constraints, causing duplicate key errors when importing matches with 3+ innings.

BEGIN;

ALTER TABLE batting_data DROP CONSTRAINT IF EXISTS batting_data_match_id_player_id_key;
ALTER TABLE bowling_data DROP CONSTRAINT IF EXISTS bowling_data_match_id_player_id_key;

COMMIT;
