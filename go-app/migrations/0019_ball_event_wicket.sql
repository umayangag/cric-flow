-- 0019_ball_event_wicket.sql
--
-- Every wicket on a delivery, not the first (IMPORT-06 in docs/AUDIT_FINDINGS.md).
--
-- A delivery can carry more than one wicket: a batter bowled and his partner retired hurt
-- on the same ball, two run outs, a side that retired its whole order out on one delivery
-- (1483765, ten wickets). ball_event held one wicket_kind and one player_out_id, so the
-- importer kept the first-listed wicket and dropped the rest -- 17 wicket records across
-- 16 deliveries in the archive. Rare, but the row that lost them was also the row the
-- rating pass reads, and a rule that says "the first one" is not a rule about cricket.
--
-- A child table rather than a second column pair, because the one delivery that needed
-- more than two is real cricket and a pair would have kept two of its ten. One row per
-- wicket in the order the file lists them; kind is the vocabulary's spelling
-- (configs/wicket_kinds.json), which the importer validates before it writes.
--
-- The two columns come off ball_event: a fact kept in two places is the defect this fixes
-- in another form. The first wicket each row held is moved into the new table before the
-- columns go, so a database migrated but not yet re-imported describes the same cricket it
-- did before -- the first wicket per delivery -- rather than none; the re-import is what
-- writes the other 17.

BEGIN;

CREATE TABLE IF NOT EXISTS public.ball_event_wicket (
    match_id bigint NOT NULL,
    innings smallint NOT NULL,
    "over" smallint NOT NULL,
    ball smallint NOT NULL,
    wicket_number smallint NOT NULL,
    kind character varying(24) NOT NULL,
    player_out_id bigint,
    CONSTRAINT ball_event_wicket_pkey PRIMARY KEY (match_id, innings, "over", ball, wicket_number),
    CONSTRAINT ball_event_wicket_ball_fkey FOREIGN KEY (match_id, innings, "over", ball)
        REFERENCES public.ball_event (match_id, innings, "over", ball),
    CONSTRAINT ball_event_wicket_player_out_fkey FOREIGN KEY (player_out_id)
        REFERENCES public.player (id)
);

COMMENT ON TABLE public.ball_event_wicket IS
    'One row per wicket on a delivery, in the order the Cricsheet file lists them';
COMMENT ON COLUMN public.ball_event_wicket.wicket_number IS
    '1-based position of the wicket on its delivery';
COMMENT ON COLUMN public.ball_event_wicket.kind IS
    'The wicket kind as configs/wicket_kinds.json spells it; the file says which kinds are the bowler''s and which are not dismissals';

INSERT INTO public.ball_event_wicket (match_id, innings, "over", ball, wicket_number, kind, player_out_id)
SELECT match_id, innings, "over", ball, 1, wicket_kind, player_out_id
FROM public.ball_event
WHERE wicket_kind IS NOT NULL
ON CONFLICT DO NOTHING;

ALTER TABLE public.ball_event
    DROP COLUMN IF EXISTS wicket_kind,
    DROP COLUMN IF EXISTS player_out_id;

COMMIT;
