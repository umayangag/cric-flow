-- 0005_match_id_from_source.sql
--
-- Match identity comes from the source file, not from a hash of the match's content.
--
-- `match_id` was `sha256(date | team1 | team2)` folded into twelve digits. That is a
-- content hash, not an identity: two sides can play twice in a day, and **309 of the
-- 22,734 files** in the current dataset share a (date, team, team) with another file.
-- Every one of those pairs collapsed onto a single `match_id`, which is why the database
-- held 22,425 matches for 22,734 files -- 309 real matches, with their squads, scorecards
-- and ball-by-ball records, were not in it.
--
-- Worse than the loss: the importer runs files concurrently and each writes its match in
-- one transaction that replaces the squad, so *which* file's match survived was decided by
-- goroutine scheduling. Two imports of the same directory disagreed on 80 `match_player`
-- rows across 14 matches. "The same dataset produces the same database" was false, which
-- makes every measurement taken from it unattributable (H-16).
--
-- Cricsheet names each file by its own match id -- 1130677.json -- and that is the only
-- match identity the source publishes; nothing inside the JSON names the match. So
-- `match_id` is now that number. 25 files are named with a prefix ("wi_211824") and cannot
-- be a bigint; those keep a hash, which now includes the file identifier, and it is logged.
-- The two spaces cannot meet: derived ids start at 100000000000 and Cricsheet's are six
-- and seven digits.
--
-- This is the second migration in a row to truncate and re-import, and for the same
-- reason: the id of every existing row is wrong under the new rule, and the source is two
-- minutes away. `player` and `opposition` are deliberately *not* truncated -- 0004 keyed
-- them by identifiers the source provides, which this change does not touch, so the
-- importer's upserts land on the same rows and the identity work is not redone.

BEGIN;

-- Everything keyed by match_id, plus the precomputed tables, which are derived from
-- matches and are empty until precompute-features runs again.
TRUNCATE TABLE
    public.ball_event,
    public.match_player,
    public.batting_data,
    public.bowling_data,
    public.fielding_data,
    public.fielding_event,
    public.match_inning,
    public.match,
    public.match_prediction_aggregates,
    public.feature_raw_stats_snapshots,
    public.player_window_features,
    public.batting_transition_features,
    public.bowling_sequence_features,
    public.bowling_spell_features,
    public.dot_streak_features,
    public.event_reaction_features,
    public.extras_discipline_features,
    public.wicket_mode_features,
    public.over_boundary_wicket_features,
    public.over_end_pressure_features
    RESTART IDENTITY;

COMMIT;
