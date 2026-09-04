-- 0010_player_biography.sql
--
-- Player biographies acquired from Wikidata (X-1a in docs/EXTERNAL_DATA_PLAN.md).
--
-- The ball-by-ball archive cannot say how old a player was, which hand he bats with, or
-- what he bowls: Cricsheet records what happened, not who it happened to. Those are the
-- facts X-1b wants to test as features, and they have to be acquired before they can be
-- gated. Wikidata carries them under CC0, joined to Cricsheet's people register through
-- the ESPNcricinfo player id both sides publish (Wikidata property P2697).
--
-- Why a table rather than a column on `player`. A biography is a *different source* with
-- a different refresh rhythm and a different failure mode: the importer rebuilds `player`
-- from the archive on every identity change (0004 truncates and re-imports), and a
-- biography that lived on that row would be destroyed by an import that knows nothing
-- about Wikidata. Keyed by player_id with its own fetch timestamp, it survives, is
-- re-derivable on its own schedule, and states plainly which pass wrote it.
--
-- Why a row exists for players nothing was found for. "We looked and Wikidata has no item
-- for him" and "we never looked" are different states, and only the first is a measured
-- coverage figure. A row with a null wikidata_qid is the first; no row is the second.
-- This is the same distinction the retirement ledger draws with Verdict.Unavailable, and
-- it is what makes the coverage report a measurement rather than a guess.
--
-- Nothing infers. A career end date is written only where Wikidata states one; an absent
-- date stays null rather than becoming "probably retired", because the corroboration
-- criteria in internal/availability read this table and a guess there would promote a
-- user's claim to a stored fact on evidence that does not exist.

BEGIN;

CREATE TABLE IF NOT EXISTS public.player_biography (
    -- The registry id is the key: one biography per player, and the FK means a player
    -- rebuilt by an identity migration takes its biography with it or loses it loudly.
    player_id bigint PRIMARY KEY,

    -- The join key actually used, kept so a match can be re-checked by hand. It comes
    -- from Cricsheet's people register (key_cricinfo), not from this database, so
    -- recording it here is the only way to explain a match after the fact.
    cricinfo_id character varying(32),

    -- The Wikidata item the join landed on. NULL means the pass ran and found no item:
    -- that is a measured miss, not a missing measurement.
    wikidata_qid character varying(32),

    -- P569. The one field with real coverage, and the one X-1b's age family needs.
    birth_date date,

    -- P741 / P552, mapped to 'left' / 'right'. Wikidata almost never carries this for
    -- cricketers; the column exists so the gap is visible as a null rather than absent.
    batting_hand character varying(16),

    -- P2545 mapped to the controlled vocabulary in internal/biography: pace, medium,
    -- off-spin, leg-spin, left-arm-orthodox, left-arm-wrist, unknown.
    bowling_style character varying(32),

    -- The label the mapping read, kept beside the mapped value. A vocabulary that hides
    -- what it was given cannot be corrected later without re-fetching everything.
    bowling_style_raw text,

    -- P2032 (end of work period). Spotty by nature -- recorded where stated, never
    -- inferred. internal/availability's career_end criterion reads exactly this.
    career_end_date date,

    -- P570. A death is not a retirement date and is deliberately not written into
    -- career_end_date; it is recorded as itself so the two are never conflated.
    death_date date,

    -- 'wikidata' or 'override'. An override is a hand-curated row from
    -- configs/player_biography_overrides.json, and it must be distinguishable from an
    -- acquired one or the coverage figure stops meaning what it says.
    source character varying(32) NOT NULL,

    -- The licence the values were acquired under, recorded per row because the answer
    -- can differ per source. Wikidata is CC0-1.0.
    source_license character varying(32),

    -- When this row was written. The ops surface reads it to say how stale the pass is;
    -- staleness is a property of the data, not of a log line nobody opens.
    fetched_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE ONLY public.player_biography
    ADD CONSTRAINT player_biography_player_id_fkey
    FOREIGN KEY (player_id) REFERENCES public.player(id);

-- The coverage report counts matched rows per format and gender, and the ops surface
-- asks the same question on every load.
CREATE INDEX IF NOT EXISTS player_biography_matched_idx
    ON public.player_biography (wikidata_qid)
    WHERE wikidata_qid IS NOT NULL;

COMMIT;
