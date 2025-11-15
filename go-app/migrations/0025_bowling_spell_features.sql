-- 0025_bowling_spell_features.sql
-- Bowling spell features: detect spells (contiguous overs) and split first over vs later overs

BEGIN;

CREATE TABLE IF NOT EXISTS bowling_spell_features (
    as_of_date DATE NOT NULL,
    format_id  INT  NOT NULL,
    scope      TEXT NOT NULL DEFAULT 'overall',
    scope_id   INT,
    scope_id0  INT  GENERATED ALWAYS AS (COALESCE(scope_id, 0)) STORED,
    player_id  BIGINT NOT NULL,
    phase      TEXT NOT NULL,

    -- spell aggregates across all spells up to as_of_date
    spells                INT    NOT NULL DEFAULT 0,
    spell_overs           INT    NOT NULL DEFAULT 0,    -- total overs across spells

    -- first over of spell totals
    first_overs_balls     INT    NOT NULL DEFAULT 0,
    first_overs_runs      INT    NOT NULL DEFAULT 0,
    first_overs_wickets   INT    NOT NULL DEFAULT 0,
    first_overs_dots      INT    NOT NULL DEFAULT 0,
    first_overs_boundaries INT   NOT NULL DEFAULT 0,

    -- later overs within spells (excluding the first)
    later_overs_balls     INT    NOT NULL DEFAULT 0,
    later_overs_runs      INT    NOT NULL DEFAULT 0,
    later_overs_wickets   INT    NOT NULL DEFAULT 0,
    later_overs_dots      INT    NOT NULL DEFAULT 0,
    later_overs_boundaries INT   NOT NULL DEFAULT 0,

    -- convenience rates
    first_over_econ       DOUBLE PRECISION NOT NULL DEFAULT 0,
    later_over_econ       DOUBLE PRECISION NOT NULL DEFAULT 0,

    PRIMARY KEY (as_of_date, format_id, scope, scope_id0, player_id, phase)
);

CREATE INDEX IF NOT EXISTS idx_bowling_spell_latest
  ON bowling_spell_features (player_id, format_id, scope, scope_id0, phase, as_of_date DESC);

COMMIT;
