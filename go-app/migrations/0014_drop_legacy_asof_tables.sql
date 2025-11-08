-- Migration 0014: drop legacy as-of snapshot tables (no longer used)
-- Note: ensure all readers/writers are migrated before applying.

DROP TABLE IF EXISTS player_form_asof;
DROP TABLE IF EXISTS player_consistency_asof;
DROP TABLE IF EXISTS player_vs_opposition_asof;
DROP TABLE IF EXISTS player_at_venue_asof;
