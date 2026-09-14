-- 0020_match_player_replacement.sql
--
-- Which member of a side came in after the match started (FEAT-02 in docs/AUDIT_FINDINGS.md).
--
-- Cricsheet lists everyone who took the field under info.players, so a side that used a
-- concussion substitute, an impact player, a supersub or a covid replacement is listed as
-- twelve (1,343 sides in the current archive), thirteen (21) or fourteen (1). Who the extra
-- man is lives elsewhere in the file: a `replacements.match` entry on the delivery he came
-- in at, naming him (`in`), the player he replaced (`out`), the side and the reason. The
-- rating pass had no way to read that from match_player, so it rated and aggregated those
-- sides as squads of twelve while the serving path always aggregates eleven -- and a
-- replacement is decided during the match, so his presence is post-start information.
--
-- A flag on the row rather than a shorter squad, because the row is a fact worth keeping:
-- he did play, his deliveries are his, and a reader that wants everyone who took the field
-- (appearances, biographies) reads the table as before. A reader that wants the eleven
-- that started reads WHERE NOT is_replacement.
--
-- NOT NULL DEFAULT false is the truth for every row of an eleven and for the eleven who
-- started an oversized side. It is not the truth for the replacement himself on a row
-- imported before this migration: those read false until the directory is re-imported,
-- which is what sets the flag, and `make xi-parity` reports the difference against the
-- archive (oversized_squads, replacement_players) until then.

BEGIN;

ALTER TABLE public.match_player
    ADD COLUMN IF NOT EXISTS is_replacement boolean DEFAULT false NOT NULL;

COMMENT ON COLUMN public.match_player.is_replacement IS
    'True for a player who joined the side after the match started -- the `in` of a Cricsheet replacements.match entry -- and so was not one of the eleven that started';

COMMIT;
