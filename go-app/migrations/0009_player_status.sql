-- 0009_player_status.sql
--
-- The retirement ledger (D-12).
--
-- `player.is_retired` has existed since the baseline schema and nothing has ever written
-- it: the candidate-pool query filtered on `is_retired = 0`, every row was 0, and the
-- filter selected everybody. The Upcoming-match pool therefore offered players who
-- retired a decade ago, and the surface said nothing, because from the query's point of
-- view no one was excluded.
--
-- The fix separates two things the single column conflated. A user saying "he has
-- retired" is a *claim*, held per user and true of that user's pools only. A stored
-- `is_retired = 1` is a *fact* about the player that every pool honours. This ledger
-- holds the claims, and a claim is promoted to the fact only when an independent
-- criterion corroborates it (go-app/internal/services/availability).
--
-- Why two tables. `player_status` is the current claim, one row per (player, user), so
-- the pool query joins it directly. `player_status_event` is the append-only history:
-- every promotion records which criterion corroborated it and when, and every demotion
-- records that it happened, so an exclusion a user sees can always be explained and
-- undone. A current-state row alone would answer "is he excluded" but never "why", and
-- "why" is the whole point of promoting on evidence rather than on assertion (§8.7).

BEGIN;

-- One user's claim about one player.
CREATE TABLE IF NOT EXISTS public.player_status (
    player_id bigint NOT NULL,

    -- Who made the claim. Deployments today carry a single API key and therefore a
    -- single actor, but the column is not thereby redundant: it is what keeps a flag
    -- scoped to the person who set it, which is the difference between the claim and
    -- the fact this table exists to draw.
    flagged_by text NOT NULL,

    flagged_at timestamptz NOT NULL DEFAULT now(),

    -- Set only when a criterion corroborated the claim and `player.is_retired` was
    -- raised. NULL means the claim stands on its own: it hides the player from this
    -- user's default pools and from nobody else's.
    promoted_at timestamptz,
    promoted_criterion text,
    promoted_detail text,

    PRIMARY KEY (player_id, flagged_by)
);

ALTER TABLE ONLY public.player_status
    ADD CONSTRAINT player_status_player_id_fkey
    FOREIGN KEY (player_id) REFERENCES public.player(id);

-- The pool query reads one user's whole claim set per request.
CREATE INDEX IF NOT EXISTS player_status_flagged_by_idx
    ON public.player_status (flagged_by);

-- Every change to a claim or to the stored fact, in the order it happened.
CREATE TABLE IF NOT EXISTS public.player_status_event (
    id bigserial PRIMARY KEY,
    player_id bigint NOT NULL,
    actor text NOT NULL,

    -- 'flagged', 'promoted', 'demoted' or 'unflagged'. Kept as text rather than an enum
    -- for the same reason the criteria are a list and not a column: X-1a adds evidence
    -- sources, and a new criterion must not need a schema change to be recorded.
    event text NOT NULL,

    -- Which criterion corroborated a promotion, and the evidence it read. NULL on a
    -- flag or an un-flag, which corroborate nothing.
    criterion text,
    detail text,

    occurred_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE ONLY public.player_status_event
    ADD CONSTRAINT player_status_event_player_id_fkey
    FOREIGN KEY (player_id) REFERENCES public.player(id);

CREATE INDEX IF NOT EXISTS player_status_event_player_idx
    ON public.player_status_event (player_id, occurred_at DESC);

COMMIT;
