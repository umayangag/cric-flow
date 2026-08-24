# Repository cleanup PR checklist

Tracked cleanup work from the reachability audit (August 2026). Implement **one PR at a time**; mark status here as work progresses.

Scope: dead code removal, retirement of CLI paths superseded by the API, removal of backfill/legacy-compatibility layers, and structural streamlining across `go-app/`, `ml-service/`, and `frontend/`.

**Status legend:** `todo` | `in_progress` | `done` | `skipped` | `blocked`

---

## Status

| ID | Status | PR branch (when done) | Title |
|----|--------|----------------------|-------|
| C0-1 | done | `cleanup/c0-1-junk-files` | Remove committed junk files and tighten `.gitignore` |
| C0-2 | done | `cleanup/c0-2-orphan-frontend-tree` | Delete the orphan frontend tree; one test location |
| C1-1 | done | `cleanup/c1-1-migration-down-files` | Fix: migration runner executes `.down.sql` as forward migrations |
| C1-2 | todo | | Fix: `make precompute` uses a non-existent API key |
| C1-3 | todo | | Remove `cmd/evaluate` scaffold |
| C1-4 | done | `cleanup/c1-4-generalized-pipeline` | Remove the unused generalized-pipeline experiment |
| C1-5 | todo | | Remove the unwired match-harmony modules |
| C1-6 | todo | | Remove pre-restructure ML leftovers |
| C1-7 | todo | | Fold `ml_service/` into `ml/` (= existing **P0-5**) |
| C1-8 | todo | | Remove the unused cricsheet importer port/adapter layer |
| C1-9 | todo | | Remove remaining orphaned Go functions |
| C2-1 | blocked | | **Decision gate:** weather — build it or delete it |
| C2-2 | blocked | | Remove the weather subsystem and its feature slots |
| C3-1 | todo | | Delete `train_batting_model` / `train_bowling_model` |
| C3-2 | todo | | Remove the `_LEGACY_` artifact tier |
| C4-1 | done | — | **Decision:** squash migrations to a baseline — **Option A approved** |
| C4-2 | done | `cleanup/c4-2-squash-migrations` | Collapse migrations into `0001_baseline.sql` |
| C5-1 | todo | | Single source of truth for canonical format codes |
| C5-2 | todo | | Resolve `train_combination_meta`'s 501 |
| C5-3 | todo | | Reconcile the Makefile pipeline with the API pipeline |
| C6-1 | todo | | Frontend: one API client |
| C6-2 | todo | | Remove the `app/` compatibility re-export shims |
| C6-3 | todo | | Split the serving image from the training image |
| C6-4 | todo | | Consolidate `.cursor/skills` and `.junie/skills` |
| C7-1 | todo | | Generate `ARCHITECTURE_MAP.md` from the real contracts |
| C7-2 | todo | | CI guardrails so dead code stops accumulating |

---

## Conventions for every PR in this list

- **Branch:** `cleanup/<id>-<slug>`, lowercase — e.g. `cleanup/c0-1-junk-files`. Never commit to `main`.
- **Commit:** Conventional Commits with a scope and the ID, e.g. `chore(ml-service): delete generalized pipeline experiment (C1-4)`.
- **One concern per PR.** If a PR grows past ~600 changed lines outside pure deletions, split it.
- **Docs in the same branch.** If a command, endpoint, or path changes, update `README.md`, `docs/`, and the relevant `Makefile` help text in the same PR (project guideline).
- **Baseline verification** (run before and after; both must pass):
  ```bash
  make check-all
  ```
  which runs `frontend-check`, `go-app-check`, `ml-service-check`, and `frontend-backend-sync-check`.
- **Coverage gates move when tests are deleted.** `go-app` enforces `COV_MIN=60`, `ml-service` enforces `COV_MIN=78`. Deleting well-covered dead code *lowers* the reported percentage. Record the before/after number in the PR body; only adjust a threshold with an explicit note saying why.
- **Update this file in the same PR:** set the row to `done` and fill in the branch name.

### Per-PR checklist template

- [ ] Branch created from up-to-date `main`
- [ ] Scope items below all complete
- [ ] `make check-all` passes
- [ ] Coverage before/after recorded in the PR body
- [ ] Docs/Makefile updated where behaviour or commands changed
- [ ] This checklist row updated to `done`

---

## Decision gates

Items needing a product decision before their PRs can be written. Everything else proceeds independently. **C4-1 is decided (Option A);** C2-1 is still open.

### C2-1 — Weather: build it or delete it

The importer writes placeholder rows (`ingest.go:630` inserts `match_id` + `session` with every measurement column NULL). Every path that would populate real values is unreachable, and there is no weather provider client anywhere in the repo.

**Correction from the C4-2 import run:** the enqueue side is *not* dead — a 21,253-file import left **21,043 rows in `weather_job`**. `db.EnqueueWeatherJob` is reachable via `cricsheet.ImportDir`; it is the drain side (`DequeueNextWeatherJob`, `MarkWeatherJobDone`, `MarkWeatherJobFailed`, `UpsertWeather`) that has no caller. So the queue fills on every import and is never consumed, which strengthens the case for a decision either way. Seven features — `temp`, `wind`, `rain`, `humidity`, `cloud`, `pressure`, `viscosity` — are therefore constant across all training rows for **six** models (~42 feature slots).

- **Option A — Build it.** Add a provider client (Open-Meteo has a free historical archive keyed by lat/lon + date, which fits the existing `venue` geocode columns), wire `EnqueueMissingWeatherJobs` into the import step, and run the queue as a pipeline step. Reuses the existing `weather_job` table and `internal/weather/service.go`.
- **Option B — Delete it.** Drop the queue, the repos, the `weather_data` measurement columns, and the seven inputs from `configs/feature_vectors.json`. Requires re-export and full retrain.

**Answer needed:** A or B. C2-2 below is written for Option B.

### C4-1 — Migrations: squash to a baseline

`0090_full_schema_restructure.sql` already truncates every fact table on the grounds that "data is reproducible". If that holds, the 46-file chain has no remaining value.

- **Option A — Squash.** Generate `0001_baseline.sql` from `pg_dump --schema-only` against a freshly-migrated database; delete the rest; document that existing dev databases must be recreated (`make dev-destroy`, which drops volumes — `dev-purge` only removes `output/`).
- **Option B — Keep the chain.** Then C1-1 (the `.down.sql` bug) is the only migration work, and the duplicate `0029` stays as a known wart.

**Decided: Option A.** C4-2 is unblocked.

Consequence for C1-1: the renumbering and `*.up.sql` → `*.sql` renames originally scoped there became wasted work, since C4-2 collapses all 46 files regardless. C1-1 was reduced to the runner fix and its regression test — worth keeping as a standing guardrail, because after the squash the runner still globs `*.sql` and a future `.down.sql` would reintroduce the same ordering bug. Deleting the five existing down files moved into C4-2.

---

## Phase 0 — Hygiene

No behaviour change. Safe to land immediately, in either order.

### C0-1 — Remove committed junk files and tighten `.gitignore`

**Why:** Files committed by accident or left over from tooling runs. None is referenced by any build, test, route, or Make target.

**Scope**

- [ ] Delete the two zero-byte files with control-character names:
      `git rm "ml-service/$(printf '\001\001')" "ml-service/$(printf '\001\003')"`
- [ ] Delete `ml-service/ml-service/` (empty `requirements.txt` in a stray nested directory)
- [ ] Delete `ml-service/uv.lock` (3-line stub; the toolchain is `pip-compile`, see `ml-service/Makefile:30`)
- [ ] Delete `tools/` (default `npm init` output, no source)
- [ ] Delete `go-app/internal/safeurl/` (package clause and a doc comment, no declarations)
- [ ] Delete `pr_gemini_reviews_94.jsonl` and `.gemini_unresolved_ids`
- [ ] Add to `.gitignore`: `.gemini_unresolved_ids`, `pr_gemini_reviews*.jsonl`, `.cursor/tmp/`
- [ ] Remove the stale `.gitignore` comment about `tools/`

**Verify**

```bash
git ls-files | grep -c '"'          # expect 0
cd go-app && go build ./... && go vet ./...
make check-all
```

**Acceptance:** no tracked file has a non-printable character in its name; `make check-all` passes; nothing else in the tree changed.

**Risk:** none.

---

### C0-2 — Delete the orphan frontend tree; one test location

**Why:** `frontend/components/`, `frontend/pages/`, and `frontend/lib/` (547 lines) are a second, orphaned frontend. No route reaches `pages/dashboard/accuracy-trend.tsx` — `src/App.tsx` has no such route and `src/main.tsx` mounts only `App`. All three directories sit outside `tsconfig.json`'s `include: ["src", …]`, so they are never typechecked. The shipping accuracy-trend view is `src/components/WorkbenchAccuracyTrendSection.tsx`.

Separately, four component tests live in `frontend/tests/components/` and reach back into `../../src/`, while every other test sits beside its component. They run under Vitest but are excluded from `tsc`.

**Scope**

- [ ] Delete `frontend/components/AccuracyTrendChart.tsx`, `frontend/components/AccuracyTrendTable.tsx`
- [ ] Delete `frontend/pages/dashboard/accuracy-trend.tsx`
- [ ] Delete `frontend/lib/api.ts`
- [ ] Move `frontend/tests/components/{OpsBadges,OpsMatrix,OpsStatusTab,OpsSuggestions}.test.tsx` to `frontend/src/components/`, fixing the `../../src/` imports to `./`
- [ ] Delete the now-empty `frontend/tests/`
- [ ] Confirm `tsconfig.json` `include` covers everything that remains; no change should be needed once the out-of-tree dirs are gone
- [ ] Remove the `/dashboard/accuracy-trend` reference from `README.md` (Accuracy Trend section) — the live path is the Workbench tab

**Verify**

```bash
cd frontend && npm run typecheck && npm run lint && npm run build && npm run test
make frontend-check
```

**Acceptance:** all four moved tests still run and pass; `npm run typecheck` now covers every `.tsx` in the package; build output unchanged.

**Risk:** low — confirm no `vite.config.ts` alias or `index.html` entry points at the deleted tree before merging.

---

## Phase 1 — Latent bugs and dead code

C1-1 and C1-2 are bug fixes; do them first. C1-4 through C1-9 are deletions and can be reordered freely.

### C1-1 — Fix: migration runner executes `.down.sql` as forward migrations

**Why:** `RunMigrationsFS` (`go-app/internal/db/migrations_fs.go:40`) collects **every** `.sql` file in the directory, sorts by filename, and applies each one it has not recorded. There is no up/down concept. On a fresh database `0091_ml_tuned_params_metrics.down.sql` executes *before* its `.up.sql`, because `"down" < "up"` alphabetically.

It survives today only because all five down files are idempotent `DROP … IF EXISTS` statements. The first down file that is not — or the first pair whose sort order differs — corrupts a fresh bootstrap.

**Scope** (reduced after C4-1 chose Option A — see that section)

- [x] In `RunMigrationsFS`, skip files matching `*.down.sql`, via an `isDownMigration` helper; log the skipped count
- [x] Add a table test to `migrations_fs_test.go` asserting rollback scripts are neither recorded as applied nor `Exec`'d at all — covering a paired down file, an orphan down file, and an uppercase `.DOWN.SQL`
- [x] Verify the test fails with the fix disabled
- ~~Delete the five down files~~ — moved to C4-2, which deletes all 46 migration files
- ~~Rename `*.up.sql` → `*.sql`~~ — moot after the squash
- ~~Renumber the duplicate `0029`~~ — moot after the squash

**Verify**

```bash
cd go-app && go test ./internal/db/ -run TestRunMigrationsFS -v
go build ./... && go vet ./...
make go-app-check
```

**Acceptance:** rollback SQL never reaches the database; the new test fails when the guard is disabled and passes with it. No migration file is renamed, so no existing database re-applies anything.

**Risk:** low as scoped. The runner now applies strictly fewer files, and the five affected files are all idempotent `DROP … IF EXISTS` that were never meant to run forward.

---

### C1-2 — Fix: `make precompute` uses a non-existent API key

**Why:** `Makefile:119` posts `X-API-Key: test-api-key`. The default key everywhere else — `docker-compose.yml`, the frontend, `README.md` — is `dev-local-key`. The target returns 401 against a default stack.

**Scope**

- [ ] Change the header at `Makefile:119` to use a variable defaulting to the real key: `API_KEY ?= dev-local-key`
- [ ] **Leave the `e2e-backtest-smoke` block alone.** Lines 298-362 also use `test-api-key`, but that block starts its own stack with `API_KEY=test-api-key` at line 301, so it is internally consistent. `precompute` is the only target that talks to a stack it did not start
- [ ] Sweep the rest of the Makefile for other hardcoded hosts or ports
- [ ] Document `API_KEY` in the Makefile `help` text

**Verify**

```bash
make dev-up
make precompute            # expect 202, not 401
curl -s -o /dev/null -w '%{http_code}\n' -X POST \
  -H "X-API-Key: dev-local-key" http://localhost:8080/precompute
```

**Acceptance:** `make precompute` returns 202 against a default `make dev-up` stack.

**Risk:** none.

---

### C1-3 — Remove `cmd/evaluate` scaffold

**Why:** `go-app/cmd/evaluate/main.go` wires a `demoRepo` whose `LoadInputs` ignores its arguments and returns four hardcoded arrays. Real evaluation runs through `POST /api/backtest/evaluate-start` and the Evaluate DB tab.

**Scope**

- [ ] Delete `go-app/cmd/evaluate/`
- [ ] Delete `go-app/internal/services/evaluate/` (`options.go`, `runner.go`, `service.go` + 3 test files)
- [ ] Remove the `evaluate` target from `go-app/Makefile:76`
- [ ] Remove `cmd/evaluate` from `docs/overview.md` and `go-app/README.md` if listed

**Verify**

```bash
cd go-app && go build ./... && go test ./...
make go-app-check
```

**Acceptance:** builds and tests pass; no reference to `cmd/evaluate` remains (`grep -rn 'cmd/evaluate' .` returns nothing).

**Risk:** low. Check whether any metric helper in `internal/services/evaluate/service.go` is worth keeping — if a real metrics function lives there, move it to `internal/eval/` rather than deleting it.

---

### C1-4 — Remove the unused generalized-pipeline experiment

**Why:** A complete second modelling stack — target encoding, RobustScaler, walk-forward CV, Brier score — that nothing calls. Not in the Makefile, not behind an endpoint, not in the pipeline step list, not in `ARCHITECTURE_MAP.md`. Unreachable from all twelve live entrypoints.

**Scope**

- [x] Delete `ml-service/ml/generalized_pipeline.py` (641 lines)
- [x] Delete `ml-service/ml/train_generalized.py` (106 lines)
- [x] Delete `ml-service/ml/ball_by_ball_loader.py`
- [x] Delete `ml-service/tests/test_generalized_pipeline.py`, `test_train_generalized.py`, `test_ball_by_ball_loader.py`
- [x] **Kept** `ml/config.py:get_pipeline_common_config()` — 14 call sites across `train_fielding`, `train_extras`, `train_win`, `train_innings`, `training_pipeline`, and `app/train_on_the_fly`
- [x] Renamed the config key it reads from `generalized_pipeline` to `pipeline_common`. **The key was absent from both `config.json` and `config.default.json`** — every caller was already getting the in-code defaults, so the rename is behaviour-preserving
- [x] Snapshot note added to `docs/audit-coding-principles-remediation.md`, whose 2025-03-11 largest-files table listed the deleted module (and two others since split by P0-2/P0-3)

**Coverage:** 80% → 79%, gate 78%. The deleted modules were covered at 84–98%, above the project average, so removing them lowers the ratio. No test was lost for surviving code: 728 → 705 passing, the difference being the deleted modules' own tests.

**Verify**

```bash
cd ml-service && .venv/bin/pytest -q
make ml-service-check
```

**Acceptance:** tests pass; `grep -rn 'generalized' ml-service/ml ml-service/app` returns nothing.

**Risk:** low. The config-key rename is the only part that touches live code — do it in the same PR so the name stops referring to a deleted module.

---

### C1-5 — Remove the unwired match-harmony modules

**Why:** These are the pieces `model-harmony-implementation-plan.md` marks `[x] done`. They were written but never wired in. The reconciliation path that *is* live runs through `reconciliation_service` and `reconciliation_adapter`, which stay.

**Scope**

- [ ] Delete `ml-service/ml/match_schema.py`, `match_aggregates.py`, `harmony_metrics.py`
- [ ] Delete `ml-service/ml/compute_harmony_realism_metrics.py`, `compute_win_coherence_metrics.py`, `analyze_reconciliation_adjustments.py`
- [ ] Delete the matching tests: `test_match_aggregates.py`, `test_harmony_metrics.py`, `test_compute_harmony_realism_metrics.py`, `test_compute_win_coherence_metrics.py`, `test_analyze_reconciliation_adjustments.py`
- [ ] **Keep** `ml/win_coherence_metrics.py` — it is imported by `app/`
- [ ] Update `model-harmony-implementation-plan.md` and `model-harmony-plan.md`: move the deleted items from `[x] done` back to `[ ]` with a note that the code was removed as unwired, or delete both plan files if the direction is abandoned
- [ ] Update `ARCHITECTURE_MAP.md` if it references any deleted module

**Verify**

```bash
cd ml-service && .venv/bin/pytest -q
make ml-service-check
```

**Acceptance:** tests pass; the plan documents no longer claim delivered work that has no code behind it.

**Risk:** low, but this is the PR most worth a second look — if any of this is about to be wired up, keep it and mark the row `skipped` with a reason instead.

---

### C1-6 — Remove pre-restructure ML leftovers

**Why:** Modules stranded by the `0090` schema restructure and the per-format model split.

`ml/queries.py` is the clearest case: MySQL backtick syntax (`` FROM `batting_data` ``) against a Postgres database, selecting from `match_details` — a table dropped by `0090_full_schema_restructure.sql`. It has a passing test file, which asserts on string constants rather than on anything that can execute.

`ml/encoders.py` duplicates `go-app/internal/selection/encodings.go`; both copies are unreachable.

**Scope**

- [ ] Delete `ml-service/ml/queries.py` + `tests/test_queries.py`
- [ ] Delete `ml-service/ml/encoders.py` + `tests/test_encoders.py`
- [ ] Delete `ml-service/ml/player_combinator.py` + `tests/test_player_combinator.py` (the live combinator is `go-app/internal/services/predictteam`)
- [ ] Delete `ml-service/ml/batting_regressor.py`, `bowling_regressor.py` + `tests/test_batting_and_bowling_regressors.py`
- [ ] Delete `ml-service/ml/calibrate.py` + `tests/test_calibrate.py`
- [ ] Delete `ml-service/ml/tuning/__main__.py` (no `python -m ml.tuning` invocation exists anywhere)
- [ ] Delete the dead Go twins: `encodeSession` and `encodeViscosity` in `go-app/internal/selection/encodings.go`
- [ ] **Keep** `ml/validate_exports.py` — it is live via `make -C ml-service validate-exports` and `validate-exports-infer`
- [ ] Remove `ARCHITECTURE_MAP.md`'s "Player combinator" section if it describes the Python module rather than the Go one

**Verify**

```bash
cd ml-service && .venv/bin/pytest -q
cd ../go-app && go build ./... && go test ./...
make check-all
```

**Acceptance:** tests pass; `make -C ml-service validate-exports` still works.

**Risk:** low.

---

### C1-7 — Fold `ml_service/` into `ml/`

**Why:** This is **P0-5** on `ml-service/docs/IMPROVEMENT_PR_CHECKLIST.md`, with one fact that raises its priority: `ml-service/Dockerfile` copies only `app/` and `ml/`. The `ml_service/` package is absent from the image entirely, so any code path reaching it would `ImportError` in production. Nothing outside tests imports it today.

**Scope**

- [ ] Decide per module whether to keep it at all: `ml_service/baselines/{batting,bowling}.py` and `ml_service/datasets/seq_reader.py` are only exercised by their own tests
- [ ] If keeping: move to `ml/baselines/` and `ml/datasets/`, update imports in `tests/test_baselines.py` and `tests/test_seq_reader.py`
- [ ] If not: delete the package and both test files
- [ ] Delete the now-empty `ml-service/ml_service/`
- [ ] Update `Makefile:374` `train-batting-baseline` and `:377` `train-bowling-baseline`, plus `ml-test`
- [ ] Update `README.md` § "ML readers and baselines"
- [ ] Mark **P0-5** done in `ml-service/docs/IMPROVEMENT_PR_CHECKLIST.md`

**Verify**

```bash
make ml-test
make train-batting-baseline && make train-bowling-baseline
cd ml-service && .venv/bin/pytest -q
docker compose build ml-service && docker compose up -d ml-service
curl -s localhost:8000/health | jq
```

**Acceptance:** two top-level Python package names remain (`app`, `ml`); the baseline Make targets still work; the container starts.

**Risk:** low–medium (matches the existing P0-5 rating).

---

### C1-8 — Remove the unused cricsheet importer port/adapter layer

**Why:** A complete ports-and-adapters layer — `Loader` / `Parser` / `Repository` / `Log` dependency injection — that nothing wires up. `cmd/cricsheet-importer/main.go` calls `cricsheet.ImportDir` directly; the API's import step does the same. `Runner`, `NewRunner`, and `IngestService.IngestDir` are unreachable, and exist only for their own tests plus generated mocks.

**Scope**

- [ ] Delete `go-app/internal/services/cricsheetimporter/runner.go` + `runner_test.go`
- [ ] Delete `IngestService` and `IngestDir` from `go-app/internal/services/cricsheetimporter/ingest.go` + `ingest_test.go`
- [ ] **Keep** `options.go` / `options_test.go` — `ParseArgs` is used by `cmd/cricsheet-importer`
- [ ] Delete `go-app/internal/db/match_repo.go` (`MatchRepo`) if nothing else references it after the above
- [ ] Regenerate mocks (`make mock`) and confirm `internal/db/mocks/MatchRepo.go` disappears
- [ ] **Keep** `internal/cricsheet/interfaces.go` and `internal/cricsheet/mocks` — still used by `internal/cricsheet/ingest_*_test.go`
- [ ] Delete `go-app/internal/jobs/` (`Source` interface + `OneShotSource`, used only by `oneshot_test.go`)

**Verify**

```bash
make mock
cd go-app && go build ./... && go vet ./... && go test ./...
make go-app-check
```

**Acceptance:** the importer still works end to end — run `make cricsheet-import DIR=data/go-app/cricsheet` and `POST /import/cricsheet` and confirm both import the same file count.

**Risk:** medium — deletes an abstraction someone may have intended to grow into. Confirm the direction before merging; if the layer is planned, mark `skipped` with the reason.

---

### C1-9 — Remove remaining orphaned Go functions

**Why:** `deadcode -test ./...` reports functions unreachable even counting test code, plus a longer tail unreachable from `main` but exercised by tests. **Distinguish the two**: several of the latter are deliberate test seams and must stay.

**Keep (intentional DI hooks for tests):** `db.SetDB`, `db.SetPoolAPI`, `cricsheet.SetCricsheetDB`, `cricsheet.SetWeatherClient`, `cricsheet.SetRunInTxFn`, `resources.SetObservationsPathForTest`.

**Scope**

- [ ] `go-app/internal/services/teamselect/pool.go` — delete the whole file plus `pool_test.go`, `pool_more_test.go`, `pool_extra_test.go`. `LoadFromCSV` / `LoadFromDB` and seven helpers are superseded by `selection.SelectTeamFromCSV`, which `runner.go:46` actually calls
- [ ] `go-app/internal/predictor/` — delete `BuildTeam` (`predict.go`), `selectTop` (`selector.go`), `parsePlayersCSV` (`csvparse.go`), `CalculateOverallPerformanceWithConfig` (`predictor.go:38`) and their tests; keep the rest of the package, which `mlclient` and `selection` use
- [ ] `go-app/internal/services/pipeline/steps.go` — delete `StepToCommand` (superseded by `pipelineRunHandler`'s own switch)
- [ ] `go-app/internal/services/exportdataset/runner.go` — delete `NewRunner`; the API uses `NewRunnerWithServices`
- [ ] `go-app/internal/db/exportqueries/training_snapshot.go` — delete `computeBattingSnapshotAtCutoff` and `computeBowlingSnapshotAtCutoff`
- [ ] `go-app/internal/db/` — delete the unreachable repo methods: `BatchSetIsWicketKeeper`, `ListMatchesByFormatDate`, `ListFieldingBefore`, `UpsertFieldingTx`, `InsertFieldingEvent`, `RecomputeFieldingAggregates`, `InsertFieldingEventsBatch`, `EnsureMatchByID`, `ExistsMatchID`, `EnsureMatchWithFormat`, `GetByName`, `GetPlayerFormFmt`, `GetPlayerVenueEffectFmt`, `GetPlayerOppositionEffectFmt`
- [ ] `go-app/internal/cricsheet/ball_event_emit.go` — delete `EmitBallEvents`
- [ ] Regenerate mocks (`make mock`) after interface changes
- [ ] Leave the weather repos alone — they belong to C2-2

**Verify**

```bash
cd go-app && go build ./... && go vet ./... && go test ./...
go run golang.org/x/tools/cmd/deadcode -test ./...   # weather cluster only, until C2-2
make go-app-check
```

**Acceptance:** `deadcode -test ./...` reports nothing outside the weather cluster; coverage recorded before/after.

**Risk:** low–medium. Split into two PRs if the `internal/db` list makes review unwieldy — the repo methods are independent of the rest.

---

## Phase 2 — Weather

Blocked on **C2-1**. Written below for Option B (delete).

### C2-2 — Remove the weather subsystem and its feature slots

**Why:** Seven features across six models are constant. Removing them shrinks every input vector, speeds every fit, and makes the model cards describe what the system actually does.

**Scope — Go**

- [ ] Delete `go-app/internal/db/repo_weather.go`, `repo_weather_backfill.go`, `repo_weather_job.go`, `repo_venue.go`
- [ ] Delete `go-app/internal/weather/`
- [ ] Remove the placeholder insert at `go-app/internal/cricsheet/ingest.go:630`
- [ ] Remove the `--placeholders-weather` and `--weather-enqueue` flags from `internal/services/cricsheetimporter/options.go` and `cmd/cricsheet-importer`
- [ ] Remove `PLACEHOLDERS` handling from `go-app/Makefile:70`
- [ ] Drop the weather columns from the export queries in `internal/db/exportqueries/{batting,bowling}.go` and `internal/services/exportdataset/`
- [ ] Remove the weather entries from `go-app/internal/features/contract.go`

**Scope — shared contract and ML**

- [ ] Remove the seven inputs per section from `configs/feature_vectors.json` (batting, bowling, fielding) and from `EXTRAS_FEATURE_COLS`, `WIN_FEATURE_COLS`, `INNINGS_FEATURE_COLS`
- [ ] Remove weather fields from `ml-service/app/models/features.py` and the request models
- [ ] Update `tests/golden/expected_headers_*.json` and re-run the parity check

**Scope — schema**

- [ ] New migration dropping `weather_job` and the `weather_data` measurement columns — or fold into `0001_baseline.sql` if C4-2 lands first
- [ ] Delete `go-app/migrations/0006_weather_job.sql` if the squash happens

**Scope — retrain and docs**

- [ ] Re-export, retrain all six models, and record before/after metrics in the PR body
- [ ] Update `ARCHITECTURE_MAP.md` model tables (see C7-1) and `docs/config-and-data.md`

**Verify**

```bash
make dev-purge && make dev-up && make migrate
make cricsheet-import DIR=data/go-app/cricsheet
make precompute-all-all-formats
make export-dataset
make -C ml-service validate-exports
python tests/golden/run_parity.py
make train-all
curl -s localhost:8000/health | jq '.loaded_batting_formats'
make check-all
```

**Acceptance:** a full pipeline run completes from an empty database; `/health` reports models loaded for every format; model metrics are recorded and no worse than baseline (constant features carry no signal, so a regression here means something else broke).

**Risk:** high — changes the feature contract end to end. This is the one PR that must be verified with a real retrain, not just a test run.

---

## Phase 3 — Legacy artifact tier

### C3-1 — Delete `train_batting_model` / `train_bowling_model`

**Why:** Two trainers per model on two different feature contracts. `ml/train_batting.py` builds its column list from `configs/feature_vectors.json` — the shared contract the Go exporter also reads. `ml/train_batting_model.py` hardcodes its own list inline and falls back to a `batting_encoded.csv` that predates the per-format split. Both run, one after the other:

```
Makefile:162   python -m ml.train_batting --all-formats && python -m ml.train_batting_model
app/training_orchestrator.py:152   from ml.train_batting_model import run_training as run_unified_batting
```

The second writes the unsuffixed `batting_model.joblib` / `batting_scaler.joblib`, which load as `_LEGACY_` and serve any request omitting `format`. A feature added to `feature_vectors.json` reaches the per-format models and silently does not reach the legacy ones — exactly the situation the cyclical-time change on `ml-service/v3-cyclical-time-features` creates.

**Scope**

- [ ] Delete `ml-service/ml/train_batting_model.py`, `train_bowling_model.py`
- [ ] Remove the `unified_batting_csv_available()` / `unified_bowling_csv_available()` branches from `app/training_orchestrator.py:150-198`
- [ ] Simplify `Makefile:162` and `:165` to the per-format call only; remove the second invocation from `Makefile:416`
- [ ] Remove the `batting_encoded.csv` / `bowling_encoded.csv` legacy fallbacks from the exporter (`ExportLegacy` in `internal/services/exportdataset/{batting,bowling}.go` and `BattingLegacyRows` / `BowlingLegacyRows` in `internal/db/exportqueries/`)
- [ ] Delete stale unsuffixed artifacts from `output/ml-service/` and document that in the PR body

**Verify**

```bash
make export-dataset && ls output/go-app/*.csv     # no unsuffixed batting_encoded.csv
make train-batting && make train-bowling
ls output/ml-service/                              # per-format artifacts only
make check-all
```

**Acceptance:** one trainer per model; no unsuffixed artifact is produced; `/health` shows per-format models loaded and `legacy_batting: false`.

**Risk:** medium — do this before C3-2 so the legacy artifacts stop being produced before the code that reads them is removed.

---

### C3-2 — Remove the `_LEGACY_` artifact tier

**Why:** Every registry in `app/artifacts.py` carries a `"_LEGACY_"` key, and the fallback is threaded through `endpoints.py`, `players.py`, `innings.py`, the `/health` payload, and the error hints. The word "legacy" appears 279 times across `ml/` and `app/`. With C3-1 landed, nothing writes those artifacts any more.

**Scope**

- [ ] Delete `_load_legacy_artifacts()` and every `"_LEGACY_"` registry write in `app/artifacts.py`
- [ ] Delete the `legacy_*` booleans from the `/health` and `/artifacts/status` payloads
- [ ] In `app/prediction_service/endpoints.py`: make `format` required on `/predict/batting`, `/predict/bowling`, `/predict/extras`, `/predict/win`; remove `resolve_model_pair`'s legacy branch and the "train legacy artifacts" hints
- [ ] Remove the `_LEGACY_` fallbacks in `app/prediction_service/players.py:63,66,67,386,499` and `innings.py:28,40`
- [ ] Remove `LEGACY_EXTRAS_FEATURE_COLS` from `ml/train_extras.py` and its use in `endpoints.py:47`
- [ ] Update `go-app/internal/mlclient` to always send `format`
- [ ] Update `frontend/src/components/HealthTab.tsx` and `src/types.ts` where the `legacy_*` fields are consumed
- [ ] Update `ml-service/README.md` endpoint docs

**Verify**

```bash
cd ml-service && .venv/bin/pytest -q
make dev-up
curl -s -X POST localhost:8000/predict/batting -H 'content-type: application/json' \
  -d '[{"format":"T20", ...}]' | jq
curl -s -X POST localhost:8000/predict/batting -H 'content-type: application/json' \
  -d '[{...no format...}]' -o /dev/null -w '%{http_code}\n'   # expect 4xx, not a legacy fallback
make check-all
```

**Acceptance:** `grep -rin '_LEGACY_' ml-service/` returns nothing; a request without `format` fails with a clear 4xx; the Health tab renders without the legacy fields.

**Risk:** medium — a behaviour change on the predict endpoints. `go-app` is the only caller and always knows the format, so the blast radius is contained; verify with a full backtest run.

---

## Phase 4 — Migrations

Blocked on **C4-1**. Written below for Option A (squash).

### C4-2 — Collapse migrations into `0001_baseline.sql`

**Scope**

- [x] Bootstrapped an empty database through the full chain, then generated the baseline with
      `pg_dump --schema-only --no-owner --no-privileges --no-comments --exclude-table=schema_migrations`
- [x] Hand-edited: dropped `\restrict` psql meta-commands (pgx cannot execute them) and session `SET` noise,
      including `set_config('search_path','')` which would leak onto the pooled connection; wrapped in `BEGIN`/`COMMIT`
- [x] Excluded `schema_migrations` — the runner creates it itself before applying anything
- [x] Restored the `match_format` seed with **explicit ids**, since `internal/formats` hardcodes `TEST=1, ODI=2, T20=3, T20I=4`,
      then `setval` past them so later inserts do not collide
- [x] Deleted the other 45 migration files, including the five `*.down.sql` left by C1-1
- [x] Rewrote `tests/fixtures/backtest/seed.sql` against the real schema (see the correction below)
- [x] Documented the migration model in `docs/config-and-data.md`

**Verify**

```bash
make dev-purge && make dev-up && make migrate
make seed-fixtures && make e2e-backtest-smoke
make cricsheet-import DIR=data/go-app/cricsheet
make precompute-all-all-formats && make export-dataset
python tests/golden/run_parity.py
make check-all
```

**Acceptance:** met. The schema diff between a full-chain database and a baseline-built one is **two lines**, both the same CHECK constraint re-rendered by Postgres:

```
< CHECK (((role)::text = ANY ((ARRAY['bat'::character varying, 'bowl'::character varying])::text[])))
> CHECK (((role)::text = ANY (ARRAY[('bat'::character varying)::text, ('bowl'::character varying)::text])))
```

Array-level cast vs element-level cast. Proven equivalent on identical truth tables (`bat`/`bowl` → true, `x`/`BAT` → false, `NULL` → null for both), and the new form is a stable fixed point — re-applying the baseline to a third database round-trips to itself with a zero-line diff. Object counts match exactly: 29 tables, 47 indexes, 14 sequences, 18 FKs, 4 checks, 2 functions, 1 trigger.

**Risk:** medium, discharged by the diff above plus a 21,253-file Cricsheet import (10.7M ball events) and a green `e2e-backtest-smoke` from an empty volume.

---

## Phase 5 — One control plane

### C5-1 — Single source of truth for canonical format codes

**Why:** `formats.CanonicalCodes()` exists, and there is a whole CI job (`frontend-backend-sync-ci.yml` + `cmd/print_canonical`) keeping the frontend aligned with it. Meanwhile the backend writes the list out by hand in twelve places, in two different orders.

**Scope**

- [ ] Route these through `formats.CanonicalCodes()`:
      `internal/services/exportdataset/options.go:75`, `internal/services/exportdataset/formats.go:34`,
      `internal/services/opsstatus/types.go:46`, `internal/services/opsstatus/exports.go:15`,
      `internal/server/backtest_handlers_data.go:36`, `internal/services/teampredictor/helpers.go:17`,
      `internal/services/teamselect/options.go:35`
- [ ] Fix the order mismatch: settle on `TEST, ODI, T20, T20I` and make every site agree
- [ ] Python: one constant, imported by `app/artifact_service.py:24`, `app/training_orchestrator.py:282`, `ml/win_features.py:42`, `ml/validate_exports.py:150`
- [ ] Extend `scripts/check-frontend-backend-sync.mjs` (or add a Go test) asserting no other literal list exists

**Verify**

```bash
grep -rn '"TEST".*"ODI"' go-app/internal ml-service/{app,ml} | grep -v canonical
cd go-app && go test ./... && cd .. && make check-all
```

**Acceptance:** one Go list and one Python list; the grep above returns nothing outside the definitions.

**Risk:** low. Watch for order-dependent behaviour in `exports.go` and the `opsstatus` matrix.

---

### C5-2 — Resolve `train_combination_meta`'s 501

**Why:** The step is offered in the UI's step list and passes the ordering gate, then responds `501` with `"step must be run from project root"` and a `make` command in the body (`pipeline_handlers.go:118`). It is the one step that cannot complete through the interface that owns every other step.

**Scope** — pick one:

- [ ] **Implement it** like the other train steps: run it in-process via `pipeline.RunJob`, or add an ML-service endpoint and use `makeMLTrainHandler`, or
- [ ] **Remove it** from `pipelineRunHandler`'s switch, from `opsstatus.CanRunPipelineStep`, and from `frontend/src/utils/pipelineSteps.ts` so the UI stops offering a dead end

**Verify**

```bash
make dev-up
curl -s -X POST -H "X-API-Key: dev-local-key" \
  localhost:8080/ops/pipeline/run/train_combination_meta -w '\n%{http_code}\n'
cd frontend && npm run test
```

**Acceptance:** either the step completes and writes `combination_meta.json`, or it no longer appears in the UI step list. No 501 either way.

**Risk:** low.

---

### C5-3 — Reconcile the Makefile pipeline with the API pipeline

**Why:** `POST /ops/pipeline/run/{step}` owns the pipeline — ordering via `opsstatus.CanRunPipelineStep`, progress over SSE, cancellation via `/ops/pipeline/stop`. The Makefile carries a complete second path for the same steps, and the two have drifted (C1-2 is one symptom).

Do this **last** in the phase, once C1-2 and C5-2 have removed the known drift.

**Scope**

- [ ] Audit each pipeline-related target against its API equivalent: `precompute`, `precompute-asof`, `precompute-all`, `precompute-all-all-formats`, `precompute-seq`, `export-dataset`, `export-off`, `export-on`, `train-*`, `ml-auto-tune`, `full-pipeline`
- [ ] For each, decide: **thin wrapper** that calls the API (preferred — one implementation, one ordering policy), or **keep as a direct CLI** with a comment saying why the API cannot serve that case
- [ ] Delete targets that are neither
- [ ] Fix `FORMAT` being assigned twice (`Makefile:110` sets `?= T20`, `Makefile:219` re-declares it as `?=`) — the second is a silent no-op
- [ ] Rewrite the `help` and `help-all` output to say plainly which targets drive the API and which run locally
- [ ] Update `README.md` § "Key workflows" and `docs/overview.md` § pipeline order

**Verify**

```bash
make help
make dev-up
# for each retained target, run it and confirm it reaches the same end state as the API step
make check-all
```

**Acceptance:** `make help` gives one obvious path per pipeline step; no target duplicates an API step with different behaviour.

**Risk:** low individually, but touches a 32 KB Makefile — split by step group (precompute / export / train) if review gets heavy.

---

## Phase 6 — Structure

### C6-1 — Frontend: one API client

**Why:** `src/api.ts` (469 lines, the `api` object used by every hook and Ops component) and `src/api/client.ts` (used only by `BacktestEvaluate` and `BacktestFilters`) both build request URLs and both own their own helpers.

**Scope**

- [ ] Pick one — `src/api/client.ts` is the better home; the directory already exists with `types.ts`
- [ ] Move the `api` object's methods across, keeping `toUpperTrim` / `isRFC3339` and the query builders
- [ ] Update every importer (11 files import `../api`)
- [ ] Merge `src/api.test.ts` into `src/api/client.test.ts`
- [ ] Reconcile `src/types.ts` with `src/api/types.ts`

**Verify**

```bash
cd frontend && npm run typecheck && npm run lint && npm run test && npm run build
make frontend-check
```

**Acceptance:** one module builds request URLs; `grep -rn "from '.*\/api'" src` resolves to a single path.

**Risk:** low.

---

### C6-2 — Remove the `app/` compatibility re-export shims

**Why:** `.junie/guidelines.md:116` — *"No need to be backward compatible. no backfilling or preserving old features."* But `app/models/__init__.py` and `app/prediction_service/__init__.py` exist to re-export from the modules they were split into, including three private names (`_sum_team_feature`, `_assemble_player_predictions`, `_resolve_prediction_model_pairs`) re-exported for callers that could import them directly. This finishes **P0-2** and **P0-3**.

**Scope**

- [ ] Update every importer of `app.models` to import from `app.models.predict` / `.backtest` / `.features` / `.reconciliation` / `.constants`
- [ ] Update every importer of `app.prediction_service` to import from `.endpoints` / `.players` / `.innings` / `.generate_match`
- [ ] Reduce both `__init__.py` files to a docstring
- [ ] Have tests import the private helpers from their real modules; drop the `_sum_team_feature` alias
- [ ] Note the completion under P0-2/P0-3 in `ml-service/docs/IMPROVEMENT_PR_CHECKLIST.md`

**Verify**

```bash
cd ml-service && .venv/bin/pytest -q && .venv/bin/ruff check .
make ml-service-check
```

**Acceptance:** neither `__init__.py` contains a re-export; tests pass unchanged in count.

**Risk:** low — mechanical, but touches many files. Land it when no other ML PR is in flight.

---

### C6-3 — Split the serving image from the training image

**Why:** `ml-service/Dockerfile` installs the full `requirements.txt` — 1,214 pinned packages including PyCaret, AutoGluon, PyTorch, TensorBoard, and Chronos — into the container that serves sklearn predictions. `requirements-ci.in` already documents the minimal set, and its own comment says the auto-tune deps are optional. Auto-tune degrades gracefully when they are absent (`auto_tune_pycaret.py:115` logs `pycaret_not_installed skip_ranking`).

**Scope**

- [ ] Add `requirements-serve.in` (minimal: fastapi, uvicorn, pydantic, numpy, pandas, scikit-learn, joblib, structlog, httpx, psycopg2-binary, python-dotenv, psutil) and compile it
- [ ] Multi-stage `Dockerfile`: a `serve` target on the minimal set and a `train` target on the full set
- [ ] Point `docker-compose.yml`'s `ml-service` at the `serve` target
- [ ] Decide how `/admin/train/*` runs: a separate `train` container, or keep the full image for local dev and use `serve` in deployment — document the choice in `docs/ml-and-training.md`
- [ ] Record before/after image sizes in the PR body

**Verify**

```bash
docker compose build ml-service
docker images | grep cric-app-ml
docker compose up -d && curl -s localhost:8000/health | jq
make e2e-backtest-smoke
```

**Acceptance:** the serving image starts, `/health` reports models loaded, backtest smoke passes, and the image is materially smaller.

**Risk:** medium — verify that `/admin/train/*` still behaves as intended under whichever split is chosen.

---

### C6-4 — Consolidate `.cursor/skills` and `.junie/skills`

**Why:** Both directories hold the same seven skills under the same names, and `diff -rq` reports every pair as differing. Two sets of instructions for one workflow will keep diverging.

**Scope**

- [ ] Diff each pair and merge into one canonical copy
- [ ] Keep one directory as the source of truth; make the other a symlink, or drop it and note the location in `.junie/README.md`
- [ ] Same treatment for `.cursor/rules` vs `.gemini/styleguide.md` if they overlap
- [ ] Add `.cursor/tmp/` to `.gitignore` (also covered by C0-1)

**Verify** — none beyond `make check-all`; this is documentation.

**Risk:** none.

---

## Phase 7 — Docs and guardrails

### C7-1 — Generate `ARCHITECTURE_MAP.md` from the real contracts

**Why:** The file opens with *"Use `@ARCHITECTURE_MAP.md` to avoid re-reading source files"*, so its errors are inherited by anyone who trusts it. It is currently wrong in three ways that matter:

- Batting is listed at **27** inputs, bowling at **26**, fielding at **15** in one place and **16** in another. `configs/feature_vectors.json` holds **42 / 42 / 17**.
- `/predict/fielding` is documented as a player-level endpoint. It does not exist in `app/main.py`.
- `match_date_unix` is listed in every model's input set — the feature replaced by cyclical encodings on `ml-service/v3-cyclical-time-features`.

**Scope**

- [ ] Add a generator (`scripts/gen-architecture-map.mjs` or a Python equivalent) that emits the model input/output tables from `configs/feature_vectors.json` plus the `*_FEATURE_COLS` constants, and the endpoint list from the FastAPI app and `go-app/internal/server/router.go`
- [ ] Keep the hand-written prose sections; generate only the tables and the route list, between marker comments
- [ ] Regenerate and commit
- [ ] Add a CI check that a regenerated map matches the committed one
- [ ] Do this **after** C2-2 and C3-2, so it captures the final contract

**Verify**

```bash
node scripts/gen-architecture-map.mjs --check
make check-all
```

**Acceptance:** every count and endpoint in the map is derived from source; CI fails if the map drifts.

**Risk:** none.

---

### C7-2 — CI guardrails so dead code stops accumulating

**Why:** Everything in this checklist accumulated because nothing fails when code stops being reachable. `golangci-lint`'s `unused` check does not catch it — it reports unexported identifiers within a package, and most of the Go findings are exported repository methods.

**Scope**

- [ ] `.github/workflows/go-app-ci.yml`: add `go run golang.org/x/tools/cmd/deadcode -test ./...`, failing on any output. Land it only after C1-9 and C2-2, or seed an allowlist for the known-remaining set
- [ ] `.github/workflows/ml-service-ci.yml`: add the AST reachability check used for this audit — seeded from `app.main` plus the `python -m ml.*` entrypoints in `training_orchestrator.py` and the Makefiles — failing on any newly unreachable module. Commit the script as `scripts/py-reachability.py`
- [ ] `go-app/Makefile:110`: pin `golangci-lint` the way `mockery` is pinned at v3.6.0, rather than `@latest`
- [ ] Document both checks in `docs/quality-and-debugging.md`

**Verify**

```bash
cd go-app && go run golang.org/x/tools/cmd/deadcode -test ./...   # expect no output
python scripts/py-reachability.py --check
make check-all
```

**Acceptance:** both checks run in CI and fail on an intentionally-orphaned function added in a scratch commit.

**Risk:** none, provided it lands after the deletions.

---

## Recommended order

1. **C0-1 → C0-2** — hygiene, no behaviour change
2. **C1-1 → C1-2** — the two latent bugs
3. **C1-3 → C1-9** — deletions, any order; C1-8 needs a direction check first
4. **C2-1** decision, then **C2-2** — the largest single change; give it its own review cycle
5. **C3-1 → C3-2** — in that order, so legacy artifacts stop being produced before their readers go
6. **C4-1** decision, then **C4-2**
7. **C5-1 → C5-2 → C5-3** — C5-3 last, once the drift it would document has been fixed
8. **C6-1 → C6-4** — independent, any order
9. **C7-1 → C7-2** — last, so they capture the final state
