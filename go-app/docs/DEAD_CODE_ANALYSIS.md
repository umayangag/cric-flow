# Go-App Dead / Outdated Code Analysis

Summary of dead or outdated code identified in the go-app codebase that can be safely removed or cleaned up without impacting functionality.

---

## 1. **Removed: Legacy feature _fmt compute functions (entire files)**

**Files removed:**
- `internal/features/features_fmt.go`
- `internal/features/consistency_fmt.go`

**Reason:** These files contained four exported functions that were **never called** anywhere in the codebase:

- `ComputeSeasonalFormFmt` – wrote to `player_form_data_fmt`
- `ComputeVenueEffectsFmt` – wrote to venue-effect _fmt tables
- `ComputeOppositionEffectsFmt` – wrote to opposition-effect _fmt tables  
- `ComputeConsistencyFmt` – wrote to consistency _fmt tables

Precompute now uses **snapshot tables** (`feature_form_snapshots`, `feature_consistency_snapshots`) via the precompute-features runner (`RunReplay` / `RunPointInTime`). The package comment in `internal/precompute/precompute.go` states: *"the legacy _fmt tables are no longer used."*

Reading of form/venue/opposition/consistency for team selection uses `db.GetPlayerFormFmt`, `GetPlayerVenueEffectFmt`, `GetPlayerOppositionEffectFmt` in `repo_selection.go`, which read from **snapshot** tables, not from the legacy _fmt tables. No tests referenced the removed functions.

**Impact:** None. Removal is safe.

---

## 2. **Optional cleanup: Unused single-player snapshot helpers in exportqueries**

**Location:** `internal/db/exportqueries/training_snapshot.go`

**Functions (currently suppressed with `//nolint:unused`):**
- `computeBattingSnapshotAtCutoff` (lines ~50–114)
- `computeBowlingSnapshotAtCutoff` (lines ~167–230)

**Reason:** The batch path uses `computeBattingSnapshotFromHistories` and the bowling equivalent; the single-player “at cutoff” helpers are never called. They were kept “for consistency with precompute-features and possible future use.”

**Recommendation:** Optional. You can remove these two functions and drop the nolint comments to reduce dead code. Functionality is unchanged since no callers exist.

---

## 3. **Verified as used (not dead)**

| Symbol / area | Used by |
|---------------|--------|
| `GetRecentMigrations`, `GetMigrationsPaginated` | `server/ops_handlers.go` |
| `GetMatchContext` | `selection/select_team_db.go` |
| `ListPlayerPoolConsistency`, `GetPlayerFormFmt`, `GetPlayerVenueEffectFmt`, `GetPlayerOppositionEffectFmt` | `selection/select_team_db.go` |
| `IsSequenceFeaturesPopulated` | `server/ops_handlers.go` |
| `NewRunnerWithServices` (exportdataset) | `server/pipeline_handlers.go` |
| Batting/Bowling Legacy & Unified rows | Export dataset pipeline and CLI (`Unified` / legacy flags) |
| `BattingFormatRows`, `BowlingFormatRows`, `BattingInferenceRows`, `BowlingInferenceRows` | Export dataset services and tests |
| `DryRun` (seqcalc) | `cmd/precompute-all/main.go`, tests |
| Router comment “Legacy evaluatedb routes removed” | Documentation only; no code removed |

---

## 4. **Removed: Data backfill / import CLIs (not used by current pipeline or frontend)**

All of the following have been **removed** to keep the codebase minimal; weather and other data flows can be re-added later when needed.

- **etl-importer** — cmd, commands, cli, services, `db/etl_repo.go`; config `EtlDir` / `DefaultEtlDir()`.
- **import-retired** / **import-keepers** — cmd, commands, cli, and `internal/csvx/` (retired.go, keeper.go).
- **weather-import**, **weather-worker**, **weather-backfill** — cmd, commands, cli, services; kept `internal/weather` (EnqueueJob only for cricsheet) and removed `process.go` and `openmeteo/`.
- **backfill-fielding** — cmd, commands, cli, `internal/services/fielding/`, `db/fielding_repo.go` and its mock.
- **backfill-ball-events** — cmd.

Makefile targets and help text for these have been removed. The API pipeline (import → precompute → export) and cricsheet import (including optional `WeatherEnqueue` and fielding recompute per match) are unchanged.

---

## 5. **Summary**

- **Done:** Removed legacy _fmt compute code (`features_fmt.go`, `consistency_fmt.go`).
- **Done:** Removed all backfill/import CLIs and related code listed in section 4.
- **Optional:** Remove `computeBattingSnapshotAtCutoff` and `computeBowlingSnapshotAtCutoff` from `training_snapshot.go` if you want to eliminate all nolint:unused code paths.
