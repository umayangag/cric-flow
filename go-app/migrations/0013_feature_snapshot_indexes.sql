-- Migration 0013: performance indexes for consolidated feature snapshot tables
-- Purpose: optimize latest-as-of lookups used by export-dataset via lateral subqueries

-- Overall scope fast-path indexes (latest-first) with INCLUDE for index-only scans
CREATE INDEX IF NOT EXISTS idx_f_form_overall_latest
  ON feature_form_snapshots (player_id, format_id, as_of_date DESC)
  WHERE scope='overall' AND scope_id IS NULL
  INCLUDE (batting_value, bowling_value, n_samples_bat, n_samples_bowl);

CREATE INDEX IF NOT EXISTS idx_f_cons_overall_latest
  ON feature_consistency_snapshots (player_id, format_id, as_of_date DESC)
  WHERE scope='overall' AND scope_id IS NULL
  INCLUDE (batting_value, bowling_value, n_samples_bat, n_samples_bowl);

-- Opposition scope fast-path (form table)
CREATE INDEX IF NOT EXISTS idx_f_form_opp_latest
  ON feature_form_snapshots (player_id, format_id, scope_id, as_of_date DESC)
  WHERE scope='opposition'
  INCLUDE (batting_value, bowling_value, n_samples_bat, n_samples_bowl);

-- Venue scope fast-path (form table)
CREATE INDEX IF NOT EXISTS idx_f_form_venue_latest
  ON feature_form_snapshots (player_id, format_id, scope_id, as_of_date DESC)
  WHERE scope='venue'
  INCLUDE (batting_value, bowling_value, n_samples_bat, n_samples_bowl);
