-- 0003_match_player.sql
--
-- The players each side picked for a match (S-3c in docs/WIN_PROB_SELECTION_PR_CHECKLIST.md).
--
-- Until now the only record of who was in a match was the scorecard: batting_data
-- holds whoever came to the crease, bowling_data whoever sent down an over. Both
-- memberships are decided by how the match went. A team that chases with wickets in
-- hand has four batters in batting_data; a team that is bowled out has eleven. The
-- win export aggregates its features over those memberships, so the number of team-2
-- batters alone predicts the result with held-out AUC 0.89-0.94 in the limited-overs
-- formats -- and 0.539 in TEST, where both sides bat their innings out regardless of
-- who wins. That gap is the leak, and it exists because the squad was never stored.
--
-- Cricsheet has carried it all along, in info.players: every one of the 22,734 match
-- files in the current dataset has it, and the team names it keys on always match
-- info.teams. The importer simply never read it.
--
-- Note the table is not named match_xi. Cricsheet lists everyone who took the field,
-- so 1,337 of 45,468 sides in the current dataset have 12 members and a few have 13
-- or 14 -- concussion and injury replacements. That is a fact about the match, not a
-- defect to normalise away, and unlike the scorecard memberships it is not a function
-- of the result.

BEGIN;

CREATE TABLE IF NOT EXISTS public.match_player (
    match_id bigint NOT NULL,
    player_id bigint NOT NULL,

    -- Which side picked them. The win export joins this against
    -- match_inning.batting_team_opposition_id to tell team1 from team2, so it has to
    -- be stored per row: player alone cannot say, since a player's team is a property
    -- of the match and not of the player.
    opposition_id bigint NOT NULL,

    -- One row per player per match. Players are identified by name here as everywhere
    -- else in this schema, so two people sharing a scorecard name are one player_id --
    -- which this key then cannot hold twice for one match. Two files in the current
    -- dataset do name the same player on both sides; the importer drops such a name
    -- from both squads rather than guessing a side, since Cricsheet's own registry is
    -- keyed by name and collapses them into one identifier too.
    PRIMARY KEY (match_id, player_id)
);

-- The export reads one side of one match at a time.
CREATE INDEX IF NOT EXISTS match_player_match_team_idx
    ON public.match_player (match_id, opposition_id);

ALTER TABLE ONLY public.match_player
    ADD CONSTRAINT match_player_player_id_fkey FOREIGN KEY (player_id) REFERENCES public.player(id);

ALTER TABLE ONLY public.match_player
    ADD CONSTRAINT match_player_opposition_id_fkey FOREIGN KEY (opposition_id) REFERENCES public.opposition(id);

-- Deliberately no foreign key to match. Matching the rest of the schema: batting_data
-- and bowling_data reference player but not match, because the importer writes a
-- match's rows in one transaction and orphans cannot outlive it.

COMMIT;
