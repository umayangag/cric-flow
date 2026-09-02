-- 0008_drop_precompute_and_tuning_tables.sql
--
-- P-6 deletes the producers: the precompute pass, the sequence-feature calculators, the
-- export CSVs, the windowed-form win model and the auto-tune stack. Every table below has
-- lost both its last writer and its last reader in that PR, which is the only condition
-- under which this project drops one.
--
-- Feature tables (precompute):
--   feature_raw_stats_snapshots  2.32M rows, 1.81M of them duplicates (D-1). Written by
--                                `precompute-features`, read by the exports and the base
--                                models. All three are gone.
--   player_window_features       rolling windows for the sequence exports, same fate.
--
-- Sequence-feature tables (seqcalc). Nine calculators, written by
-- `precompute-sequence-features` and read only by the sequence exports and one repo. E1
-- kept no family, so nothing is re-implemented as an as-of accumulator inside the rating
-- pass; the calculators go with their tables.
--
-- Tuning:
--   ml_tuned_params              the Optuna / PyCaret / AutoGluon parameter store. What
--                                replaces it is the run manifest (H-16): the hyperparameters
--                                a run chose live in runs/<id>/manifest.json, beside the
--                                artifacts they produced, rather than in a table nothing
--                                could join back to an artifact.
--
-- Weather:
--   weather_data, weather_job    nothing has ever populated them (docs/weather-not-implemented.md).
--                                Their readers were the ops probe, the training-snapshot
--                                export and the win contract's seven constant columns; all
--                                three die here.
--
-- What survives and why: batting_data, bowling_data, fielding_data and fielding_event are
-- still written by the importer, which P-6 keeps. §9.1 has them going once nothing writes
-- them either; that is not this PR.
--
-- Forward only, as every migration here is. Nothing derived from these tables is kept, so
-- there is nothing to migrate -- only to stop producing.

DROP TABLE IF EXISTS feature_raw_stats_snapshots;
DROP TABLE IF EXISTS player_window_features;

DROP TABLE IF EXISTS batting_transition_features;
DROP TABLE IF EXISTS bowling_sequence_features;
DROP TABLE IF EXISTS bowling_spell_features;
DROP TABLE IF EXISTS dot_streak_features;
DROP TABLE IF EXISTS event_reaction_features;
DROP TABLE IF EXISTS extras_discipline_features;
DROP TABLE IF EXISTS over_boundary_wicket_features;
DROP TABLE IF EXISTS over_end_pressure_features;
DROP TABLE IF EXISTS wicket_mode_features;

DROP TABLE IF EXISTS ml_tuned_params;

DROP TABLE IF EXISTS weather_data;
DROP TABLE IF EXISTS weather_job;
