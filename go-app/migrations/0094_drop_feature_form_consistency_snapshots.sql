-- Drop legacy form/consistency snapshot tables; all consumers now use feature_raw_stats_snapshots.

DROP TABLE IF EXISTS feature_form_snapshots CASCADE;
DROP TABLE IF EXISTS feature_consistency_snapshots CASCADE;
