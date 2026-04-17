-- Backfill fielding_data.inning_number from fielding_event.
--
-- Migration 0095 added inning_number with DEFAULT 1 for all existing rows, which
-- silently collapses every historical fielding stat onto inning 1. Where we have
-- authoritative per-event data in fielding_event, we can recompute the aggregates
-- split by inning. For matches with no fielding_event data we leave the existing
-- rows untouched (they will remain at inning 1 until re-ingested).
--
-- Manually entered fields (dropped_catches, missed_run_outs) are NOT tracked in
-- fielding_event. To avoid losing them, we stash them and attach them back to the
-- inning-1 row only (we cannot know which inning they belong to without richer
-- telemetry).

BEGIN;

-- If 0095 ran before it dropped uq_fielding_match_player, that legacy UNIQUE (match_id, player_id)
-- still blocks multiple innings per player; remove it so per-inning inserts can succeed.
ALTER TABLE fielding_data DROP CONSTRAINT IF EXISTS uq_fielding_match_player;

CREATE TEMP TABLE _fielding_manual_stash ON COMMIT DROP AS
SELECT
    fd.match_id,
    fd.player_id,
    fd.dropped_catches,
    fd.missed_run_outs
FROM fielding_data fd
WHERE EXISTS (SELECT 1 FROM fielding_event fe WHERE fe.match_id = fd.match_id)
  AND (fd.dropped_catches IS NOT NULL OR fd.missed_run_outs IS NOT NULL);

-- Only remove rows we will replace from fielding_event aggregates. Players present only in
-- fielding_data (no fielding_event rows as that fielder) keep their manually entered stats.
DELETE FROM fielding_data fd
WHERE fd.inning_number = 1
  AND EXISTS (
    SELECT 1
    FROM fielding_event fe
    WHERE fe.match_id = fd.match_id
      AND fe.fielder_id = fd.player_id
);

WITH aggregated_events AS (
    SELECT
        fe.match_id,
        fe.innings AS inning_number,
        fe.fielder_id AS player_id,
        SUM(CASE WHEN fe.kind = 'caught'  THEN 1 ELSE 0 END) AS catches,
        SUM(CASE WHEN fe.kind = 'run_out' THEN 1 ELSE 0 END) AS run_outs,
        SUM(CASE WHEN fe.kind = 'stumped' THEN 1 ELSE 0 END) AS stumpings,
        SUM(CASE WHEN fe.kind = 'run_out' AND fe.is_direct_hit THEN 1 ELSE 0 END) AS runouts_direct_hits
    FROM fielding_event fe
    WHERE fe.fielder_id IS NOT NULL
    GROUP BY fe.match_id, fe.innings, fe.fielder_id
)
INSERT INTO fielding_data (
    match_id,
    inning_number,
    player_id,
    catches,
    run_outs,
    stumpings,
    runouts_direct_hits,
    dropped_catches,
    missed_run_outs
)
SELECT
    agg.match_id,
    agg.inning_number,
    agg.player_id,
    agg.catches,
    agg.run_outs,
    agg.stumpings,
    agg.runouts_direct_hits,
    CASE WHEN agg.inning_number = 1 THEN s.dropped_catches END,
    CASE WHEN agg.inning_number = 1 THEN s.missed_run_outs END
FROM aggregated_events agg
LEFT JOIN _fielding_manual_stash s ON s.match_id = agg.match_id AND s.player_id = agg.player_id
ON CONFLICT (match_id, inning_number, player_id) DO NOTHING;

-- Rows for matches without fielding_event data stay at inning_number = 1.
-- Operators can run RecomputeFieldingAggregates for any remaining matches
-- (see go-app/internal/db/repo_fielding_event.go) if per-inning event data
-- becomes available later.

COMMIT;
