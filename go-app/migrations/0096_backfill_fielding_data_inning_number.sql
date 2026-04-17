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

CREATE TEMP TABLE _fielding_manual_stash ON COMMIT DROP AS
SELECT
    fd.match_id,
    fd.player_id,
    fd.dropped_catches,
    fd.missed_run_outs
FROM fielding_data fd
WHERE EXISTS (SELECT 1 FROM fielding_event fe WHERE fe.match_id = fd.match_id)
  AND (fd.dropped_catches IS NOT NULL OR fd.missed_run_outs IS NOT NULL);

DELETE FROM fielding_data fd
WHERE EXISTS (SELECT 1 FROM fielding_event fe WHERE fe.match_id = fd.match_id);

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
    fe.match_id,
    fe.innings AS inning_number,
    fe.fielder_id AS player_id,
    SUM(CASE WHEN fe.kind = 'caught'  THEN 1 ELSE 0 END) AS catches,
    SUM(CASE WHEN fe.kind = 'run_out' THEN 1 ELSE 0 END) AS run_outs,
    SUM(CASE WHEN fe.kind = 'stumped' THEN 1 ELSE 0 END) AS stumpings,
    SUM(CASE WHEN fe.kind = 'run_out' AND fe.is_direct_hit THEN 1 ELSE 0 END) AS runouts_direct_hits,
    CASE WHEN fe.innings = 1 THEN (
        SELECT s.dropped_catches FROM _fielding_manual_stash s
        WHERE s.match_id = fe.match_id AND s.player_id = fe.fielder_id
    ) END AS dropped_catches,
    CASE WHEN fe.innings = 1 THEN (
        SELECT s.missed_run_outs FROM _fielding_manual_stash s
        WHERE s.match_id = fe.match_id AND s.player_id = fe.fielder_id
    ) END AS missed_run_outs
FROM fielding_event fe
WHERE fe.fielder_id IS NOT NULL
GROUP BY fe.match_id, fe.innings, fe.fielder_id
ON CONFLICT (match_id, inning_number, player_id) DO NOTHING;

-- Rows for matches without fielding_event data stay at inning_number = 1.
-- Operators can run RecomputeFieldingAggregates for any remaining matches
-- (see go-app/internal/db/repo_fielding_event.go) if per-inning event data
-- becomes available later.

COMMIT;
