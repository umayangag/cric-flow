-- Migration 0015: drop per-format pre-aggregated *_fmt tables (legacy)
-- These tables are superseded by consolidated feature snapshot tables and unified exporters.
-- Safe to drop once code paths no longer reference them.

DROP TABLE IF EXISTS player_form_data_fmt;
DROP TABLE IF EXISTS player_consistency_data_fmt;
DROP TABLE IF EXISTS player_venue_data_fmt;
DROP TABLE IF EXISTS player_opposition_data_fmt;
