-- 0004_identity.sql
--
-- Player and team identity (P-1 in docs/ML_PIPELINE_REARCHITECTURE_PLAN.md; I-3/I-4 in
-- docs/IDENTITY_PR_CHECKLIST.md).
--
-- Until now a player *is* a name string and a team *is* a name string. Measured against
-- the 22,734 files in the current dataset, restricted to people who appear in a squad:
--
--   163 names cover 348 different people   -> 348 careers blended into 163 player rows
--    44 people are spelled two ways        -> 44 careers split across 88 player rows
--   130 of 394 team names are used by both a men's and a women's side
--
-- Cricsheet ships the fix for the player half: info.registry.people maps every name in a
-- file to a stable person identifier, and it resolves both directions at once, which no
-- name-normalisation heuristic can. Coverage in the current dataset is total -- every name
-- used in a squad or a delivery has a registry entry -- but the column stays nullable
-- because a source gap must fall back to name-keying and be logged, not lose the match.
--
-- The team half needs no new source: match.gender already carries it, so identity becomes
-- (opposition_name, gender). This settles *identity* only. Whether the models should also
-- be split by gender is a separate, deliberate question (open decision 1 in the identity
-- checklist); today they are mixed by accident of the schema rather than by choice.
--
-- Franchise lineage (opposition.canonical_id, I-4) is deliberately NOT here: it is a
-- hand-reviewed mapping of nine renames, and it needs this migration's gender split to
-- exist first before its detection rule finds Royal Challengers.

BEGIN;

-- Rebuild, do not backfill.
--
-- Re-pointing the existing rows in place would mean rewriting 20 player-bearing columns
-- across 17 tables, and for a name that covers two people it would need the per-match
-- source to say which person each row belongs to. That information is only in the JSON,
-- which makes the backfill a re-import wearing a disguise. The import takes ~2 minutes
-- and this project has no backward-compatibility requirement, so the match-derived rows
-- go and the importer writes them again under the new keys.
--
-- The precomputed feature tables go with them: every one is keyed by player_id, so after
-- a re-key their rows describe people who no longer exist. They are rebuilt by
-- precompute-features, and P-6 deletes them outright. Leaving them populated would let
-- the legacy base models keep serving features for the pre-migration ids in silence,
-- which is exactly the failure this migration exists to end.
--
-- One statement so Postgres accepts the group: every table referencing another in the
-- list is in the list. RESTART IDENTITY so a re-import assigns ids from 1 again and two
-- runs of the same dataset are comparable row for row.
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
    public.over_end_pressure_features,
    public.player,
    public.opposition
    RESTART IDENTITY;

-- The Cricsheet registry identifier: 8 hex characters today, sized for room.
ALTER TABLE public.player ADD COLUMN external_id character varying(32);

-- Which match date the stored display name came from. A person's name changes -- the
-- registry id f3a18a0c is "NR Sciver" in 283 squads and "NR Sciver-Brunt" in 171 -- and
-- the one to show is the current one, not the most frequent one, whose winner flips as
-- matches accumulate. Nullable for a row created before any date is known.
ALTER TABLE public.player ADD COLUMN name_as_of date;

-- The name is now a display attribute, not an identity, so it cannot be unique: that
-- constraint is precisely what merged the 348 people into 163 rows.
ALTER TABLE public.player DROP CONSTRAINT IF EXISTS player_player_name_key;

-- Nullable-unique: Postgres treats NULLs as distinct, so every player with a registry
-- entry is one row and the fallback rows below are unconstrained by it.
CREATE UNIQUE INDEX IF NOT EXISTS player_external_id_key
    ON public.player (external_id);

-- The fallback path keeps its old behaviour, scoped to itself: when the source has no
-- registry entry a name is the best identity available, and one row per such name is
-- still right. Partial so it never fights the index above.
CREATE UNIQUE INDEX IF NOT EXISTS player_name_without_external_id_key
    ON public.player (player_name) WHERE external_id IS NULL;

-- Team identity gains gender. NOT NULL with no default: a team row without a gender is
-- the ambiguity this column exists to remove, and every match file in the dataset
-- carries info.gender, so there is nothing to default for.
ALTER TABLE public.opposition ADD COLUMN gender character varying(10) NOT NULL;

ALTER TABLE public.opposition DROP CONSTRAINT IF EXISTS opposition_opposition_name_key;

ALTER TABLE public.opposition
    ADD CONSTRAINT opposition_name_gender_key UNIQUE (opposition_name, gender);

COMMIT;
