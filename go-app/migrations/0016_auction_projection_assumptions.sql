-- 0016_auction_projection_assumptions.sql
--
-- The projection's named assumptions (P3-2).
--
-- A projection of a candidate's output is conditional on three things, and at an auction
-- not one of them is a fact: the eleven he would join, the opposition that eleven would
-- face, and the grounds. 0015 already holds the grounds (`auction_venue`). These two
-- tables hold the other two, on the auction record rather than in a request, for one
-- reason: every item after this one -- replacement level (P3-3), the value-vs-price flag
-- (P3-4) and the re-ranking (P3-5) -- reads "the buyer's likely eleven", and a list that
-- lived in each request would let three surfaces disagree about what the eleven was while
-- all three showed numbers labelled with it.
--
-- What these are: guesses the operator typed. `auction_likely_xi` is the buyer's squad so
-- far plus whoever they expect to fill the rest; `auction_opposition_player` is a side
-- they expect to play, usually seeded from a real recent eleven and then edited. Every
-- answer made under them carries them back, so a projection on screen says which guess it
-- was made for.
--
-- What they are not: a fielded eleven. `match_player` holds elevens that actually played;
-- nothing here ever will, because the fixture these assume does not exist. Nothing here is
-- a prediction either, so nothing here reaches `issued_prediction` or the track record
-- (roadmap § 6, clause 6).
--
-- Why the opposition carries a side and not only eleven names. The performance model reads
-- a ground only through the team context, and `ml.xi.rows.team_context_or_neutral` falls
-- back to neutral for the *pair* -- taking the venue with it -- whenever either side is
-- unnamed. An opposition with no side would therefore make every ground read identically,
-- and the venue mix this whole item exists for would be three copies of one row.

BEGIN;

-- The eleven a candidate would be projected into. Ten with a place open for him is the
-- usual state and eleven naming him already is the other; which it is only matters once a
-- candidate is named, so no size is fixed here.
CREATE TABLE IF NOT EXISTS public.auction_likely_xi (
    auction_id  uuid   NOT NULL REFERENCES public.auction (id) ON DELETE CASCADE,
    player_id   bigint NOT NULL REFERENCES public.player (id),
    -- The order the operator listed them in, kept so the surface reads back what was
    -- typed. It is not a batting order: nothing in this system predicts one, and the
    -- performance model reads a player's own sequence vectors and not his place in a list.
    position    integer NOT NULL,
    PRIMARY KEY (auction_id, player_id)
);

COMMENT ON TABLE public.auction_likely_xi IS
    'The eleven a candidate would be projected into, as the operator guessed it (P3-2). An assumption, never a fielded eleven';

-- The side the projection is against.
CREATE TABLE IF NOT EXISTS public.auction_opposition (
    auction_id     uuid   PRIMARY KEY REFERENCES public.auction (id) ON DELETE CASCADE,
    opposition_id  bigint NOT NULL REFERENCES public.opposition (id)
);

COMMENT ON TABLE public.auction_opposition IS
    'The side a projection is against (P3-2). Named by the operator; a projection against no one is refused rather than made neutral';

-- Its eleven. A whole eleven or nothing: unlike the likely eleven there is no candidate
-- joining it, so a ten-man opposition is a side nobody plays.
CREATE TABLE IF NOT EXISTS public.auction_opposition_player (
    auction_id  uuid    NOT NULL REFERENCES public.auction (id) ON DELETE CASCADE,
    player_id   bigint  NOT NULL REFERENCES public.player (id),
    position    integer NOT NULL,
    PRIMARY KEY (auction_id, player_id)
);

COMMENT ON TABLE public.auction_opposition_player IS
    'The opposition eleven a projection is against, as the operator named it (P3-2)';

COMMIT;
