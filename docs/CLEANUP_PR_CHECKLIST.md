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
| C1-2 | done | `cleanup/c1-2-precompute-api-key` | Fix: `make precompute` uses a non-existent API key |
| C1-3 | done | `cleanup/c1-3-remove-evaluate-scaffold` | Remove `cmd/evaluate` scaffold |
| C1-4 | done | `cleanup/c1-4-generalized-pipeline` | Remove the unused generalized-pipeline experiment |
| C1-5 | done | `cleanup/c1-5-match-harmony` | Remove the unwired match-harmony modules |
| C1-6 | done | `cleanup/c1-6-pre-restructure-leftovers` | Remove pre-restructure ML leftovers |
| C1-7 | done | `cleanup/c1-7-fold-ml-service` | Fold `ml_service/` into `ml/` (= existing **P0-5**) |
| C1-8 | done | `cleanup/c1-8-importer-port-layer` | Remove the unused cricsheet importer port/adapter layer |
| C1-9 | done | `cleanup/c1-9-orphaned-go-funcs` | Remove remaining orphaned Go functions |
| C2-1 | done | — | **Decision:** weather — **remove features now, keep schema**, build later |
| C2-2a | done | `cleanup/c2-2a-weather-plumbing` | Remove the dead weather plumbing (no contract change) |
| C2-2b | todo | | Remove the 7 weather features from the contract (unblocked by C2-2b-pre) |
| C2-2b-pre | done | `test/c2-2b-header-alignment` | Export header-alignment tests — the guard C2-2b needs |
| C3-1 | done | `cleanup/c3-1-drop-unified-trainers` | Delete `train_batting_model` / `train_bowling_model` |
| C3-2 | done | `cleanup/c3-2-remove-legacy-registry` | Remove the `_LEGACY_` artifact tier |
| C4-1 | done | — | **Decision:** squash migrations to a baseline — **Option A approved** |
| C4-2 | done | `cleanup/c4-2-squash-migrations` | Collapse migrations into `0001_baseline.sql` |
| C5-1 | done | `cleanup/c5-1-canonical-formats` | Single source of truth for canonical format codes |
| C5-2 | todo | | Resolve `train_combination_meta`'s 501 |
| C5-3 | todo | | Reconcile the Makefile pipeline with the API pipeline |
| C6-1 | done | `cleanup/c6-1-one-api-client` | Frontend: one API client |
| C6-2 | done | `cleanup/c6-2-drop-app-shims` | Remove the `app/` compatibility re-export shims |
| C6-3 | done | `cleanup/c6-3-split-serving-image` | Split the serving image from the training image |
| C6-4 | done | `cleanup/c6-4-generate-junie-skills` | Consolidate `.cursor/skills` and `.junie/skills` |
| C7-1 | todo | | Generate `ARCHITECTURE_MAP.md` from the real contracts |
| C7-2 | done | `ci/c7-2-guardrails` | CI guardrails so dead code stops accumulating |
| C7-3 | done | `test/c7-3-seqcalc-coverage` | Raise coverage on live under-tested code to absorb deletions |

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

**Decided: neither A nor B as written — a third shape.** Weather is wanted later, but the data does not exist yet, and getting it is a real project rather than a quick win: **0 of 877 venues have coordinates** after a full import, so a geocoding pass is needed before any weather fetch.

The features go now, the schema stays. The deciding argument: models trained on constant-zero weather **cannot** use real weather later — a full retrain is mandatory whenever the data arrives. So keeping the slots to "avoid churn later" buys nothing (re-adding seven names to a JSON file is a 7-line diff), while costing today:

| model | weather inputs | share of vector |
|---|---|---|
| extras | 7 / 15 | 47% |
| fielding | 7 / 17 | 41% |
| innings | 7 / 17 | 41% |
| win | 7 / 21 | 33% |
| batting | 7 / 42 | 17% |
| bowling | 7 / 42 | 17% |

Exports emit `COALESCE(w.temp, 0)`, so every one is a literal `0.0` in every training row.

Kept for the future: the `weather_data` and `weather_job` tables (already in `0001_baseline.sql`) and the `venue` geocode columns. The build plan is written up in [weather-not-implemented.md](weather-not-implemented.md).

**Split into two PRs** because the second needs a full retrain: **C2-2a** removes the dead plumbing with no contract change; **C2-2b** removes the seven features, which requires a re-export and retraining all six models.

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

- [x] Change the header at `Makefile:119` to use a variable defaulting to the real key: `API_KEY ?= dev-local-key`, plus `API_URL ?= http://localhost:8080`
- [x] Add `-fsS` to the curl. **This is the more important half:** the old command had no `--fail`, so a 401 returned **curl exit code 0** and the target reported success while doing nothing

**Worse than the plan described.** Verified against a running stack:

```
old:  curl -X POST -H "X-API-Key: test-api-key" ...   -> HTTP 401 Unauthorized, exit 0
new:  curl -fsS -X POST -H "X-API-Key: dev-local-key" -> HTTP 202 started,      exit 0
```

Because the failure was silent, `make up-all` (step 4/7) and `make full-pipeline` both continued past a precompute that never ran, then exported and trained on features that were never computed. `-fsS` means a future key mismatch fails loudly instead.
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

**Risk:** low.

**On the metric helpers the plan asked about:** they were already in the right place. `services/evaluate/service.go` was a thin wrapper whose `ComputeMetrics` just delegated to `internal/eval.MAE/RMSE/BrierScore`. Nothing needed moving; the wrapper went and `internal/eval` stayed.

**`internal/eval` is now caller-less — deliberately kept.** It is a 100%-covered, dependency-free implementation of MAE, RMSE and Brier score. Meanwhile the live backtest path reimplements absolute error inline as `math.Abs(pred - act)` in roughly eight places (`backtest_services.go`, `backtest_handlers.go`, `backtest_handlers_helpers.go`, `services/backtest/metrics.go`). Deleting a correct implementation while the codebase open-codes the same idea elsewhere would be the wrong trade, and it would also drop coverage to ~59.8%, under the gate.

**Follow-up worth doing:** route the backtest metrics through `internal/eval` rather than inline `math.Abs`. That turns a caller-less package into the single metrics implementation, and is real consolidation rather than deletion.

**Coverage:** 60.1% → 60.0% (gate 60). `services/evaluate` was ~95% covered.

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
- [x] `model-harmony-implementation-plan.md` gets a correction banner rather than per-item edits: eight `[x] done` items cite the deleted modules, and the honest framing is that `[x]` there means **designed and prototyped**, not in use. `model-harmony-plan.md` needed no change — it references none of them.
- [x] `ARCHITECTURE_MAP.md` references none of the deleted modules

**This was the "second look" the plan asked for, and it changed the shape of the PR.** Three docs described the deleted code as live capability:

- **`docs/match-schema.md`** — 401 lines defining "the canonical internal representation ... that all models and reconciliation logic must satisfy". Deleting it would have thrown away real domain knowledge (how extras affect bowling figures, the deterministic accounting rules). It was also *already* dangling: it cited `ml.ball_by_ball_loader`, deleted back in C1-4. **Kept, reframed as a specification**, with a status banner pointing at the live `ml/reconciliation_*` modules.
- **`docs/rollout-reconciliation.md`** — listed two deleted CLIs as operational steps.
- **`docs/stage3-joint-modelling.md`** — pointed at `ml.harmony_metrics` for realism bands.

Leaving those would have repeated exactly the failure the audit opened with: docs promising tools that no longer exist.

**`ml/win_coherence_metrics.py` stays** — `app/prediction_service/generate_match.py` imports it. Only the `compute_*` CLI wrapper around it went.

**Removed the six `pending C1-5` entries** from `scripts/py-reachability.py`'s allowlist; the check still passes.

**Coverage/tests:** 648 → 628 passing (the 20 were the deleted modules' own tests). ~997 lines of module code removed.

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

- [x] Delete `ml-service/ml/queries.py` + `tests/test_queries.py`
- [x] Delete `ml-service/ml/encoders.py` + `tests/test_encoders.py`
- [x] Delete `ml-service/ml/player_combinator.py` + `tests/test_player_combinator.py` (the live combinator is `go-app/internal/services/predictteam`)
- [x] Delete `ml-service/ml/batting_regressor.py`, `bowling_regressor.py` + `tests/test_batting_and_bowling_regressors.py`
- [x] Delete `ml-service/ml/calibrate.py` + `tests/test_calibrate.py`
- [x] Delete `ml-service/ml/tuning/__main__.py` (confirmed: no `python -m ml.tuning` invocation anywhere)
- [x] Delete the dead Go functions `encodeSession` / `encodeViscosity` (`go-app/internal/selection/encodings.go`) + test file
- [x] **Kept** `ml/validate_exports.py` — live via `make -C ml-service validate-exports` and `validate-exports-infer`
- [x] `ARCHITECTURE_MAP.md`'s "Player combinator" section describes the **Go** implementation, so it stays; C7-1 regenerates that file anyway

**Corrections found during execution**

- The plan called the Go `encodeSession`/`encodeViscosity` "duplicates" of `ml/encoders.py`. They are not the same functions: Python maps strings (`"Excellent"` → 3, `"day"` → 0), Go clamps and binarises ints. Both were dead, but for independent reasons.
- `ml/queries.py` is dead beyond doubt: `match_details` now appears **0 times** in `0001_baseline.sql`, so it queried a table that no longer exists in any form — in MySQL backtick syntax, against Postgres.
- **`go-app/coverage-func.txt` was tracked** — a 468-line generated coverage report committed in March. Removed and gitignored; `make coverage-func` regenerates it.
- `docs/ml-and-training.md` advertised `ml.calibrate` with a function list. Rewritten to say calibration is not implemented and to point at `ml/train_win.py` as the right place for it, rather than leaving the doc promising a deleted module.
- Noted for C6-2: `ml/auto_tune.py` opens with *"Backward-compatible shim — all logic lives in ml.tuning package"* — another re-export shim in the same family as the `app/` ones.

**Coverage:** ml-service 79% (gate 78%), go-app 60.2% (gate 60). 705 → 672 passing, the difference being the deleted modules' own tests.

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

- [x] **Decision: move, not delete.** They are not test-only — `make train-batting-baseline` / `train-bowling-baseline` import them via `python -c`, and the root README documents them as tooling.
- [x] Moved to `ml/baselines/` and `ml/datasets/`; imports rewritten across sources, tests and the Makefile
- [x] `ml-service/ml_service/` gone — two top-level Python packages remain, not three
- [x] Updated the baseline Make targets (see below), `ml-service/README.md`'s layout table, and marked **P0-5** done

**The move alone fixes the Docker half of P0-5.** `Dockerfile` already does `COPY ml-service/ml ./ml`, so the packages now ship. Verified in the built image rather than assumed:

```
$ docker run --rm <image> python -c "import ml.baselines, ml.datasets"
moved packages import inside the image
```

**Three pre-existing broken targets, fixed here.** All three are documented in the root README, and all three failed before this change:

| target | was | now |
|---|---|---|
| `train-batting-baseline` | `NameError: name '__file__' is not defined` — `python -c` has no `__file__` | `batting baseline trained: 3 rows 10 features` |
| `train-bowling-baseline` | same | `bowling baseline trained: 3 rows 10 features` |
| `ml-test` | `pytest: command not found` — bare `pytest`, not the venv's | `4 passed` |

Confirmed against `origin/main` that this predates the move. It also mattered: the "script entrypoint" justification for keeping `ml.baselines` is only honest if the scripts actually run.

**A modelling flaw in the C7-2 script, caught by the check itself.** Allowlisting a module suppresses its own error but does **not** make it a graph root, so everything it uniquely imports still reads as dead — moving `ml.baselines` in immediately produced four false positives. Fixed properly: `SCRIPT_ENTRYPOINTS` are now roots alongside `MODULE_ENTRYPOINTS`, and `ALLOWED_UNREACHABLE` is a last resort.

**Result: 79 of 79 modules reachable, empty allowlist.** The reachability gate now enforces a genuinely clean baseline rather than a grandfathered one. All four failure modes re-verified after the restructure.

**Tests:** 628 passing (unchanged by the move).

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

- [x] Delete `go-app/internal/services/cricsheetimporter/runner.go` + `runner_test.go`
- [x] Delete `ingest.go` (`IngestService`) + `ingest_test.go` — the whole file was the dead service, not just the method
- [x] **Kept** `options.go` / `options_test.go` — `ParseArgs` is used by `cmd/cricsheet-importer`. These are now the package's only files.
- [x] Delete `go-app/internal/db/match_repo.go` + `internal/db/mocks/MatchRepo.go`
- [x] Delete `internal/cricsheet/interfaces.go` (`Loader`, `Parser`) + their mocks — **correction to the plan**, see below
- [x] **Kept** `internal/cricsheet/deps.go`'s `CricsheetDB` + `mocks/CricsheetDB.go` — that is what `internal/cricsheet/ingest_*_test.go` actually uses
- [x] Delete `go-app/internal/jobs/`
- [x] Drop `Loader`, `Parser`, `MatchRepo` from `.mockery.yml`

**Correction to the plan:** it said to keep `internal/cricsheet/interfaces.go` "still used by `internal/cricsheet/ingest_*_test.go`". Wrong — those tests use `MockCricsheetDB`, which comes from the `CricsheetDB` interface in `deps.go`, not from `Loader`/`Parser`. `MockLoader` and `MockParser` were referenced only by the two deleted `cricsheetimporter` test files, so the ports went with the layer.

**Also removed:** `internal/models/match.go`, `weatherdata.go`, and `forecast.go`. `models.Match` existed only for the deleted `Parser`/`MatchRepo` signatures; `WeatherData` and `Forecast` had zero references and were leftovers from the weather work. All three are in `internal/models`, which `COVERAGE_EXCLUDE` omits, so they do not affect the ratio.

**Acceptance met.** Both import paths agree on a 25-file subset against a database built from `0001_baseline.sql`:

| path | matches |
|---|---|
| `go run ./cmd/cricsheet-importer -in=…` | 25 |
| `POST /import/cricsheet` | 25 (plus 569 `batting_data`, 20,691 `ball_event`) |

**Coverage:** 61.5% → 61.0% (gate 60).

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
- [x] Leave the weather repos alone — they belonged to C2-2, already done

**Result: `deadcode -test ./...` now reports zero findings for go-app.**

**Cascades the plan did not anticipate** — each surfaced only after the primary deletion:

- `internal/predictor/{predict,selector,csvparse}.go` were left as bare `package` + import blocks once their single function went, so the files were deleted outright along with their tests. `predictor.PlayerPrediction` has 25 external references and stays.
- `insertBallEventsFn` in `cricsheet/deps.go` lost its only caller with `EmitBallEvents` and began failing the `unused` linter.
- Four test files referenced deleted functions and were trimmed: `pipeline/steps_test.go` (`TestStepToCommand`), `teamselect/select_more_test.go` (`TestLoadFromCSV_…`), plus `predictor_test.go` deleted whole.
- `exportdataset/runner_test.go` used `svc.NewRunner()`. Since that was only `&Runner{}`, the tests now construct the struct directly — the coverage is kept without keeping a dead constructor.

**Noted, not done:** `db.InsertBallEvents` is production-dead — the live path uses `InsertBallEventsTx` — but it is exercised by its own unit and integration tests, so `deadcode -test` does not flag it. Out of this PR's scope; worth folding into C7-2's guardrail discussion.

**Coverage:** 61.0% → 60.1% (gate 60), close to the −0.7pt projected in C7-3.

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

### C2-2b-pre — Export header-alignment tests

**Why:** C2-2b's risk is a same-count column reordering in the export queries, which nothing caught. `exportqueries/headers.go` only covered sequence columns, and `tests/golden/expected_headers_*.json` had rotted (see below).

**Scope**

- [x] Extracted all 11 inline `headers := []string{…}` literals into named accessors in `exportqueries/export_headers.go`, so they can be asserted without a database
- [x] `export_headers_test.go` with four checks: contract tracking, no duplicates, per-format vs cross-format agreement, and stable counts
- [x] Proved the extraction changed nothing: all **331 columns across 11 lists** compared byte-identical before and after
- [x] Proved the tests fail on each failure mode, rather than assuming they would

| injected fault | caught by |
|---|---|
| same-count reorder (**the silent one**) | "columns are out of contract order" |
| contract feature dropped from the export | "set of contract features missing … changed" |
| duplicated column | "column count changed (16 → 17)" |

**Two findings that changed the design**

1. **My first assertion encoded a false invariant.** I asserted the `*_infer_*.csv` headers must equal `configs/feature_vectors.json`. They must not: the export omits the 4 cyclical time features and the 8 sequence features. Those are not a bug — sequence columns are appended only via `AppendSeqIfEnabled`, and the cyclical ones are derived at prediction time. **Live serving is unaffected**, because it goes through `ComputeFeaturesAtCutoff*`, whose `ensureContractKeys` fills every contract key by construction. The test now asserts the *documented* omission set instead, so adding a contract feature without extending the export fails loudly.

2. **The inference and training exports use different names for the same features on purpose** — `batting_temp`/`venue` in inference (matching `feature_vectors.json`), `temp`/`batting_venue` in training (matching `ml/train_batting.py:FEATURE_COLS`). A well-meaning "let's make these consistent" edit would break training silently. Pinned by the tests.

**Noted, not fixed:** `tests/golden/expected_headers_*_training.json` are stale — 22 columns including `batting_consistency` and `season_id`, versus the ~41 the export actually emits since the raw-windowed-stats migration. They are consumed by `make -C ml-service validate-exports --use-golden`, which is not in CI, which is why they rotted. Same pattern as the `Makefile` path-filter gap in C7-2.

---

### C2-2b — scope reassessment (2026-08-26)

**Attempted, then backed out deliberately.** The shared contract and `internal/features/contract.go` edits are trivial; the exporter is not. This is materially larger than every other item in this checklist and warrants being planned rather than pushed through.

**What the surface actually is**

| file | weather-bearing query variants | SELECTs in file |
|---|---|---|
| `exportqueries/batting.go` | 5 | 21 |
| `exportqueries/bowling.go` | 5 | 24 |
| `exportqueries/fielding.go` | 2 | 2 |
| `exportqueries/extras.go` | 1 | 13 |
| `exportqueries/innings.go` | 1 | 19 |
| `exportqueries/win.go` | 1 | 18 |

15 variants, and the weather columns appear in at least five distinct shapes: single-column `COALESCE(w.x, 0) AS name`, multi-column `COALESCE(...)` lines, bare `w.temp, w.wind, …` inside multi-line SELECTs, multi-line `CASE WHEN lower(w.viscosity) …` expressions, and `GROUP BY` clauses. A first pass with pattern matching caught roughly half and left the files inconsistent, so it was reverted.

**Why that matters more than the line count.** Rows are read with `scanx.ScanToStrings(rows, len(headers))`.

*Correction to an earlier draft of this note:* an off-by-**count** edit is **not** silent — `ScanToStrings` builds `n` destinations and pgx errors when that does not match the column count, so the export fails loudly. The silent case is narrower but real: removing one column from the SELECT and a *different* one from the headers keeps the count equal and shifts every column in between. That produces training data that looks fine and is wrong.

**C2-2b-pre now guards exactly that** — see below.

**Also in scope, not yet touched**

- `EXTRAS_FEATURE_COLS`, `WIN_FEATURE_COLS`, `INNINGS_FEATURE_COLS` in `ml/train_{extras,win,innings}.py`
- Weather fields on `app/models/features.py` and the predict/backtest request models
- `weather_composite` in `ml/match_level_derived_features.py` (weights in `ml/config.py`, consumed by `app/reconciliation.py`) — a derived feature computed from `rain`, `humidity`, `cloud`, all constant 0
- `tests/golden/expected_headers_*.json` and `tests/golden/run_parity.py`
- Re-export and retrain of all six models

**Recommended approach when this is picked up**

1. **Land a header-alignment test first.** Nothing today asserts that an exporter's header list matches `configs/feature_vectors.json`. `exportqueries/headers.go` only covers sequence columns. With that test in place the silent failure becomes a loud one, and the SQL edits can proceed safely — it is also worth having permanently.
2. Then the exporter edits, verified by running a real export and diffing emitted headers against the contract.
3. Then the ML col lists, request models and golden headers.
4. Then re-export and retrain, comparing metrics.

Step 1 has standalone value and could land on its own.

---

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

- [x] Delete `ml-service/ml/train_batting_model.py`, `train_bowling_model.py`
- [x] Remove the `unified_*_csv_available()` branches from `app/training_orchestrator.py`
- [x] Simplify the Makefile training targets (three call sites, not two — `Makefile:398` also invoked the legacy trainers)
- [x] Remove `ExportLegacy` + `BattingLegacyRows` / `BowlingLegacyRows` from the exporter and repo
- [x] `ResolveFormats` no longer falls back to `[""]`; it returns `formatsPkg.CanonicalCodes()`, so exports are always per-format

**Found during execution — a third legacy producer the plan missed**

`ml/training_pipeline.py:_run_legacy_csv` trained from the unsuffixed `<model>_encoded.csv` and saved with `format=None`, which is exactly what the `_LEGACY_` registry loads. So the modern trainer had its own path to unsuffixed artifacts. With the exporter no longer writing those CSVs, the path was unreachable except via an explicit `--csv`; removed, and "no formats resolved" is now a clear error rather than a silent fallback.

**`*_encoded_all.csv` stays.** It looked like part of the same legacy family, but `train_fielding`, `train_innings`, `train_win`, `train_extras` and `ml/tuning/cli.py` all read it — those models are not format-split. Only the *unsuffixed* `<model>_encoded.csv` was legacy.

**Two pre-existing bugs fixed in passing**

- **`make mock` was broken.** `.mockery.yml` carried two entries pointing at `internal/commands/...`, a directory tree that no longer exists after a rename to `internal/services/...`. Mockery aborted on the first one, so mocks could not be regenerated at all. One entry was a duplicate of a correct one and was deleted; the other had its path corrected to `internal/services/exportdataset`.
- Unified exports now also emit per-format CSVs in the no-config case, because `ResolveFormats` returns real formats instead of `[""]`. `runner_test.go` was asserting the old behaviour.

**Left alone, noted for C5-3:** `cfg.Export.SplitByFormat` no longer changes any outcome — both branches now resolve to the canonical formats. It is referenced in two handlers and the config schema, so removing it belongs with the wider config reconciliation.

**Coverage:** go-app 60.1% (gate 60); ml-service 670 passing.

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
- [x] Update `ml-service/README.md`, `docs/config-and-data.md` and `docs/ml-and-training.md`

**Behaviour change:** `/predict/batting`, `/predict/bowling`, `/predict/extras` and `/predict/win` now return **400 `MISSING_FORMAT`** when `format` is absent, instead of silently serving an unsuffixed model. A format with no loaded model still returns 404 `MODEL_NOT_LOADED`. Both go-app (`models.BattingFeatures.Format`) and the ML request models already carry `format` per row, so no client change was needed — the failure mode simply became explicit.

**A name collision worth knowing about.** `_LEGACY_` had *two* unrelated meanings. The artifact-registry key is gone. The other survives: `ml/train_extras.py`, `ml/train_fielding.py` and `ml/tuning/*` use `_LEGACY_` as a dict key for **pooled cross-format training rows**, popped and handled specially by the loaders. That is data pooling, not an artifact tier, and touching it would risk extras/fielding training. Left alone, documented in `docs/ml-and-training.md`, and worth renaming later.

**Frontend:** the Health tab's "Legacy (unified)" row and the five `legacy_*_available` fields in `types.ts` are gone.

**Coverage:** go-app 60.1% (gate 60); ml-service 655 passing (down from 670 — the 15 removed were tests of the deleted fallback).

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

**Acceptance:** met. One Go list (`formats.CanonicalCodes()`) and one Python list (`ml.config.CANONICAL_FORMAT_CODES`, via `get_format_codes()`), and all six consumers agree:

```
Go   print_canonical : ["TEST","ODI","T20","T20I"]
py   config          : ['TEST', 'ODI', 'T20', 'T20I']
py   artifact_service: ['TEST', 'ODI', 'T20', 'T20I']
py   auto_tune       : ['TEST', 'ODI', 'T20', 'T20I']
py   win_features    : ['TEST', 'ODI', 'T20', 'T20I']
check-frontend-backend-sync.mjs: OK
```

**Added `formats.IsCanonical()`.** Three of the Go sites tested membership with a `switch` or a `map`, not iteration, so a list alone could not replace them. Covered by table tests including a guard that `CanonicalCodes()` and `IsCanonical()` cannot drift apart, and one confirming aliases are rejected until `CanonicalizeCode` is applied.

**Order changed in ops status.** `opsstatus` used `TEST, ODI, T20I, T20`; everything now uses `TEST, ODI, T20, T20I`. This affects iteration order in `/ops/status` sections — cosmetic, but visible in the UI.

**Python duplication was worse than the plan counted.** Beyond the two plain constants, four modules (`training_pipeline`, `win_features`, `validate_exports`, `tuning/cli`) each carried a near-identical helper reading `ml.formats` from config with its own copy of the fallback. `win_features._load_format_codes()` is gone entirely, replaced by the shared getter; its public `get_format_codes()` wrapper is unchanged for callers.

**The literal-list guard moves to C7-2.** A grep-based test for stray lists is brittle; it belongs with the other CI guardrails rather than as a unit test here.

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

**The plan assumed a merge. It was a deletion.**

`src/api/client.ts` was not a second client in use — it was the base of an **orphaned cluster**, the same pattern as the `frontend/pages/` tree removed in C0-2:

```
src/pages/BacktestPage.tsx           <- nothing: no route in App.tsx, referenced nowhere
src/components/BacktestFilters.tsx   <- BacktestPage + its own test
src/components/BacktestEvaluate.tsx  <- BacktestPage + its own test
src/components/BacktestResults.tsx   <- BacktestPage + its own test
src/api/client.ts                    <- those three + its own test
src/api/types.ts                     <- those three + BacktestPage + client.ts
```

`src/api.ts` does not import `src/api/types.ts`; the two trees were entirely separate. The live evaluate flow is `/evaluate` → `EvaluateDbTab` → `useEvaluateDb` → `api` (`src/api.ts`) → `src/types.ts`.

`BacktestPage` is a 50-line prototype; the shipping flow (`EvaluateDbTab` + `EvaluateDbSection` + `useEvaluateDb`) is 844 lines. Merging a dead prototype into the live client would have been the wrong move.

- [x] Delete the orphaned cluster — **1,183 lines**
- [x] `src/api/` removed; `src/api.ts` is now the only client, with every importer resolving to it
- [x] Five endpoints that had two implementations (`/ops/suggestions`, `/api/options/{formats,teams-by-format,opponents}`, `/ops/migrations`) now have one
- [x] No reconciliation of `src/types.ts` against `src/api/types.ts` was needed — the duplicate type file went with the cluster

**The build proves it never shipped.** Bundle size is unchanged at `index-*.js 327.88 kB` before and after: tree-shaking had already excluded the whole cluster.

**Tests:** 18 files / 92 tests → 14 / 67. The 4 files and 25 tests removed were the cluster's own.

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

- [x] Rewrote **35 import statements** across `app/`, `ml/` and `tests/` to target the real submodules. Done mechanically: an AST pass built a symbol→submodule map from each package's members, then regrouped every `from app.models import A, B, C` by where the symbols actually live.
- [x] Both `__init__.py` files reduced to a 7-line docstring, **zero imports**
- [x] `tests/test_prediction_service_unit.py` now imports `sum_team_feature` from `app.prediction_service.innings`; the `_sum_team_feature` alias is gone. `_assemble_player_predictions` / `_resolve_prediction_model_pairs` come from `app.prediction_service.players` directly.
- [x] Marked P0-2/P0-3 complete in `ml-service/docs/IMPROVEMENT_PR_CHECKLIST.md`

**The plan undercounted the blast radius.** It listed `app/` and tests. `ml/` also imported `app.models` in five places — `reconciliation_service`, `reconciliation_core`, `reconciliation_adapter`, `consistency_eval`, `win_features_from_reconciled` — and those only surfaced when the shim was emptied and the app failed to import.

That is **P0-1's `ml` ↔ `app` cycle** in concrete form: `ml/` reaching into `app.models` for Pydantic DTOs. Not fixed here (P0-1 wants a neutral `contracts` package), but the imports are now explicit about which module they cross into, which makes that refactor easier to scope.

**Tests:** 628 passing, unchanged — this is a pure import rewrite.

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

- [x] **Renamed `requirements-ci.{in,txt}` → `requirements-serve.{in,txt}`** rather than adding a third file. That set already *was* the serving set; naming it `-ci` while the production image used it would have been misleading. CI installs it too, so tests run against exactly what the serve image ships. Regenerated with `pip-compile` rather than editing the provenance header — only one pin moved (`greenlet` added transitively).
- [x] Multi-stage `Dockerfile` with `serve` and `train` targets
- [x] `docker-compose.yml` pinned to `target: serve`
- [x] Documented in `docs/ml-and-training.md`

**5.01 GB → 1.23 GB, a 75% reduction.** Both targets built and exercised, not just built:

| | size | verified |
|---|---|---|
| `serve` | **1.23 GB** | `/health` responds; `ml.train_batting`, `ml.auto_tune`, `ml.tuning.runners` all import; `_HAS_PYCARET`/`_HAS_AUTOGLUON` correctly `False` |
| `train` | 5.02 GB | `autogluon`, `shap`, `torch` all import |

**No decision was needed on `/admin/train/*`.** The plan asked whether training needs a separate container. It does not: `/admin/train/*` shells out to `python -m ml.train_*` (scikit-learn) and `/admin/train/auto-tune` runs `ml.auto_tune` (Optuna) — both in the serving set. AutoGluon and SHAP sit behind guarded imports with graceful fallbacks, so auto-tune degrades to Optuna-only. `train` is only needed for AutoGluon's model ranking.

**⚠ PyCaret has never worked on this project's Python.** PyCaret 3.3.0 raises at import on Python ≥ 3.12 — *"Pycaret only supports python 3.9, 3.10, 3.11"* — and both the Docker image (`python:3.12-slim`) and the local venv are 3.12. It installs, pulls a large dependency tree, and is rejected every time; `_HAS_PYCARET` is `False` in both. `ml/auto_tune_pycaret.py` is guarded so nothing breaks — the cost is dead weight in the `train` image and a feature the docs implied worked.

**I could not remove it here:** `make compile-requirements-docker` fails on a `setup.py egg_info` step, and it fails on **unmodified** input too, so that target is separately broken. Two follow-ups worth their own items — fix the compile target, then drop `pycaret` (or pin the image to Python 3.11 if PyCaret ranking is wanted).

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

**The plan's premise was wrong, and following it would have broken a tool.**

`.cursor/skills` and `.junie/skills` are not redundant copies — they are the same seven workflows in **two different file formats** for two live assistants. `.cursor` uses YAML frontmatter; `.junie/README.md` explicitly specifies a plain trigger line, *"a `SKILL.md` file starting with `When the user says "/my-command" ...`"*. Both are in use: there is a `.github/workflows/junie.yaml`, and `.junie/guidelines.md` is the project's directives file. Merging into one file, or symlinking, breaks whichever tool loses its format.

**The real problem was drift, and it was behavioural.** Every pair differed (up to 137 changed lines), and not just in wording: `.junie`'s `/run-check-all-incremental` still described **three** components and omitted the `frontend-backend-sync-check` step that CI enforces. Anyone running it in Junie skipped a check.

- [x] `scripts/sync-junie-skills.py` generates `.junie/skills/` from `.cursor/skills/` — strips the frontmatter, builds Junie's trigger line from the frontmatter `description`, and stamps a "generated, do not edit" header
- [x] `make sync-skills` / `make sync-skills-check`; the check exits 1 on drift and on a Junie skill with no Cursor source
- [x] All 7 regenerated; the missing sync step is now present
- [x] `.junie/README.md` documents that the files are generated and that new skills start in `.cursor/`
- [x] `.cursor/tmp/` was gitignored in C0-1

**Not done:** `.cursor/rules` vs `.gemini/styleguide.md` do not overlap — the former is Cursor rule files, the latter a 737-byte style note for Gemini review. Nothing to consolidate.

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

- [x] `make -C go-app deadcode` wraps `deadcode -test ./...` and fails on any output; wired into `go-app-ci.yml`. Go's baseline is **0**, so it landed clean with no allowlist
- [x] `scripts/py-reachability.py` + `make -C ml-service check-reachability`, wired into `ml-service-ci.yml` **and** into `make -C ml-service ci`
- [x] Pin `golangci-lint`. CI already pinned **v2.11.1**; only the local install used `@latest`, so a developer's lint could disagree with CI. Now `GOLANGCI_VERSION ?= v2.11.1` in `go-app/Makefile`, with both sites cross-referenced. (A pre-existing duplicate `GOLANGCI_VERSION ?= latest` was removed.)
- [x] Document both in `docs/quality-and-debugging.md`
- [x] Add the root `Makefile` as a trigger path for both workflows — it matched no workflow's globs, which is why the broken `make precompute` (C1-2) went unnoticed

**The guardrail found real dead code on its first run.** `ml/dataset_definitions.py` was imported only by `ml/train_batting_model.py`, which C3-1 deleted — a cascade I missed. Removed here, along with its test.

**The Python check fails on three things, each verified by deliberately breaking it:**

| condition | verified |
|---|---|
| a new unreachable module | added a scratch module → `exit=1` |
| a stale `ALLOWED_UNREACHABLE` entry (now reachable, or gone) | listed `ml.config` → `exit=1` |
| an `ENTRYPOINTS` module that no longer exists | added `ml.renamed_away` → `exit=1` |
| baseline | `exit=0` |

The second and third matter as much as the first: a stale allowlist entry would silently mask a module coming back to life, and a stale entrypoint would make live modules look dead and invite deleting them.

**Allowlist seeded with the known backlog.** Eleven modules are tagged `pending C1-5` / `pending C1-7` so the check catches *new* dead code now rather than waiting for those items. They are self-cleaning: the check fails if a listed module becomes reachable, so entries cannot rot.

**Known blind spot, documented not hidden.** `deadcode -test` treats test files as roots, so production-dead code that has tests still passes — deliberate, since flagging test seams like `db.SetDB` would be wrong. `db.InsertBallEvents` is a live example (the real path is `InsertBallEventsTx`). Recorded in `docs/quality-and-debugging.md`.

**Still open, noted in the doc:** `configs/` is not a trigger path, so a change to `configs/feature_vectors.json` — the shared feature contract — runs no checks.

**Verify**

```bash
cd go-app && go run golang.org/x/tools/cmd/deadcode -test ./...   # expect no output
python scripts/py-reachability.py --check
make check-all
```

**Acceptance:** both checks run in CI and fail on an intentionally-orphaned function added in a scratch commit.

**Risk:** none, provided it lands after the deletions.

---

### C7-3 — Raise coverage on live under-tested code

**Why:** Deleting well-tested dead code lowers the coverage ratio even though no surviving code loses a test. Rather than ratcheting `COV_MIN_GO` down each time, buy headroom by covering **live** code that is genuinely under-tested.

The distinction matters: uncovered code that is also *unreachable* should be deleted, not tested — and deleting uncovered dead code *raises* the ratio. Only reachable code is a legitimate test target.

**What the numbers said**

At 60.0% with a 60% gate, ~59 covered statements buys one point. Of 2,377 uncovered statements, the biggest reachable blocks were:

| package | uncovered | cov |
|---|---|---|
| `internal/seqcalc` | 581 | 45.1% |
| `internal/services/predictteam` | 417 | 24.3% |
| `internal/services/opsstatus` | 280 | 52.3% |
| `internal/services/precomputefeatures` | 243 | 15.9% |

All 17 remaining truly-dead functions sit in `internal/db` / `internal/db/exportqueries`, which `COVERAGE_EXCLUDE` already omits — so C1-9 barely moves the ratio.

**Scope**

- [x] Table tests for the pure aggregation logic in `internal/seqcalc/player_rolling.go`: `makeBatAgg`, `makeBowlAgg`, `updateBatState`, `updateBowlState`, `buildRowsForInnings` — all previously at 0%
- [x] Chosen on merit, not for the metric: these compute the sequence features in the ML contract, and carry real edge cases (dots key on `RunsTotal` not `RunsBatter`; a dismissal counts only when `PlayerOutID` is the striker; boundaries conceded count only off the bat; zero balls must not divide)

**Result:** go-app 60.2% → **61.6%**; `internal/seqcalc` 45.7% → **53.8%**.

**Projected effect of the remaining deletions** (measured, not estimated):

| | stmts | covered | delta |
|---|---|---|---|
| C1-8 `cricsheetimporter/runner.go` + `internal/jobs` | 39 | 36 | −0.21pt |
| C1-9 `teamselect/pool.go` | 61 | 60 | −0.39pt |
| C1-9 `predictor` orphans | 49 | 46 | −0.28pt |
| C1-9 `EmitBallEvents` (0% covered) | 76 | 0 | **+0.17pt** |

Net roughly −0.7pt, leaving ~60.9%. **`COV_MIN_GO` does not need to move.**

**Next targets if more headroom is wanted:** `internal/services/predictteam` (417 uncovered, 24.3%) is the largest gap and covers core team-prediction logic — the 1,306-line `predict_team.go`. Higher value than seqcalc, but needs mocks rather than pure-function tests.

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
