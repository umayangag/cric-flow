-- Migration: consolidated feature snapshot tables (one table per feature, separate batting/bowling columns)

-- Exponentially Weighted Mean (form) snapshots per player/date/format with scopes
CREATE TABLE IF NOT EXISTS feature_form_snapshots (
    id BIGSERIAL PRIMARY KEY,
    player_id   BIGINT      NOT NULL REFERENCES player(id),
    as_of_date  DATE        NOT NULL,
    format_id   BIGINT      NOT NULL REFERENCES match_format(id),
    scope       TEXT        NOT NULL CHECK (scope IN ('overall','venue','opposition')),
    scope_id    BIGINT      NULL, -- NULL for overall; venue_id for 'venue'; opposition_id for 'opposition'

    batting_value REAL      NOT NULL,
    bowling_value REAL      NOT NULL,

    alpha           REAL      NOT NULL, -- EWM alpha used
    n_samples_bat   REAL      NULL,
    n_samples_bowl  REAL      NULL,
    effective_n     REAL      NULL,

    source_version TEXT     NOT NULL DEFAULT 'v1',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(player_id, as_of_date, format_id, scope, scope_id)
);

CREATE INDEX IF NOT EXISTS idx_feature_form_player_fmt_date
    ON feature_form_snapshots(player_id, format_id, as_of_date);
CREATE INDEX IF NOT EXISTS idx_feature_form_scope
    ON feature_form_snapshots(scope, scope_id, player_id, as_of_date);


-- Last-N window (consistency) snapshots per player/date/format with scopes
CREATE TABLE IF NOT EXISTS feature_consistency_snapshots (
    id BIGSERIAL PRIMARY KEY,
    player_id   BIGINT      NOT NULL REFERENCES player(id),
    as_of_date  DATE        NOT NULL,
    format_id   BIGINT      NOT NULL REFERENCES match_format(id),
    scope       TEXT        NOT NULL CHECK (scope IN ('overall','venue','opposition')),
    scope_id    BIGINT      NULL, -- NULL for overall; venue_id for 'venue'; opposition_id for 'opposition'

    batting_value REAL      NOT NULL,
    bowling_value REAL      NOT NULL,

    window_n       INTEGER  NOT NULL,
    n_samples_bat  INTEGER  NULL,
    n_samples_bowl INTEGER  NULL,

    source_version TEXT     NOT NULL DEFAULT 'v1',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(player_id, as_of_date, format_id, scope, scope_id)
);

CREATE INDEX IF NOT EXISTS idx_feature_consistency_player_fmt_date
    ON feature_consistency_snapshots(player_id, format_id, as_of_date);
CREATE INDEX IF NOT EXISTS idx_feature_consistency_scope
    ON feature_consistency_snapshots(scope, scope_id, player_id, as_of_date);
