-- 0023_extras_discipline_features.sql
-- Creates extras_discipline_features for bowler extras discipline snapshots
-- Keying convention follows other feature tables: latest-as-of snapshots keyed by
-- (as_of_date, format_id, scope, scope_id0, player_id, phase)

BEGIN;

CREATE TABLE IF NOT EXISTS extras_discipline_features (
    as_of_date DATE NOT NULL,
    format_id  INT  NOT NULL,
    scope      TEXT NOT NULL DEFAULT 'overall',   -- overall/team/league as needed
    scope_id   INT,
    scope_id0  INT  GENERATED ALWAYS AS (COALESCE(scope_id, 0)) STORED,
    player_id  BIGINT NOT NULL,                   -- bowler id
    phase      TEXT NOT NULL,                     -- powerplay/middle/death/all

    -- counts (denominators are balls_bowled for legal; wides_nb included in separate counts)
    overs              INT    NOT NULL DEFAULT 0,
    balls_bowled       INT    NOT NULL DEFAULT 0, -- legal balls only
    runs_conceded      INT    NOT NULL DEFAULT 0,
    wickets            INT    NOT NULL DEFAULT 0,

    wides              INT    NOT NULL DEFAULT 0,
    no_balls           INT    NOT NULL DEFAULT 0,
    byes               INT    NOT NULL DEFAULT 0,
    leg_byes           INT    NOT NULL DEFAULT 0,
    penalty_runs       INT    NOT NULL DEFAULT 0,

    -- convenience aggregates
    extras_total       INT    NOT NULL DEFAULT 0, -- wides + no_balls + byes + leg_byes + penalty_runs

    -- rates (stored as DOUBLE PRECISION; safe to recompute but stored for convenience)
    wides_per_over     DOUBLE PRECISION NOT NULL DEFAULT 0,
    no_balls_per_over  DOUBLE PRECISION NOT NULL DEFAULT 0,
    extras_per_over    DOUBLE PRECISION NOT NULL DEFAULT 0,

    PRIMARY KEY (as_of_date, format_id, scope, scope_id0, player_id, phase)
);

CREATE INDEX IF NOT EXISTS idx_extras_disc_latest
    ON extras_discipline_features (player_id, format_id, scope, scope_id0, phase, as_of_date DESC);

COMMIT;
