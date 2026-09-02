# Quality and debugging

Go unit testing standards, mandatory quality steps (check-all, coverage, CI, hooks, branch protection, PR template), and container crash/OOM diagnosis.

---

## Go unit testing standards

These standards define the gold-standard unit test style for the Go codebase. All new and refactored tests must follow this guidance; [../go-app/docs/testing-guidelines.md](../go-app/docs/testing-guidelines.md) has the same rules with a worked example per rule, and CLAUDE.md § Testing is the short form.

**Core principles:**

- Table-driven tests with subtests via `t.Run` and `t.Parallel()` at the test level.
- External test packages (`package xxx_test`) to validate the public API and reduce coupling.
- Clear 3-phase structure per subtest: Arrange → Act → Assert.
- Declare all mock interactions in the Arrange phase; avoid hidden side effects.
- Deterministic, small fixtures near the test; avoid randomness and `time.Now()`.
- Fail-fast assertions with `require` for critical checks; prefer `require` over `assert` unless non-fatal.
- No branching in test logic (vary cases via table entries).
- Name tests by unit and pattern, e.g. `TestService_Import_Table`.

**Mocks:** Use generated mocks under the package’s `internal/mocks` directory. Fresh mocks per subtest. Define `EXPECT()` in Arrange; assert call counts when relevant (`AssertNumberOfCalls`).

**Canonical skeleton:** One test function with `t.Parallel()`, a table of cases named `testCases` with `name`, inputs, `arrange` (ctx, mocks), `assert` (t, got, err, mocks). Loop by index — `for i := range testCases` — and instantiate the SUT inside `t.Run(tc.name, ...)`: Arrange, Act, Assert. Use `t.Helper()` for helpers.

**Naming & organization:** Test files `*_test.go` next to code. Single top-level test per primary behavior; clear case names. Helpers small and pure.

**Assertions:** Use `require.NoError`, `require.Error`, `require.ErrorContains`. Assert return values and mock call counts when side-effects matter.

**Concurrency:** Use `t.Parallel()` at the start of the test function; in subtests only if mocks/fixtures are isolated and no global state is mutated.

**Commands:** `cd go-app && go test -race -cover ./...`; for a package `go test -race -cover ./internal/services/...`; coverage report: `go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out`.

---

## H-24 — literals that cross a service boundary

**The rule:** every literal that crosses a service boundary — step ids, id kinds, date
formats, endpoint paths, query parameter names, format codes, error codes the other side
matches on — is declared **once** in a generated contract, and **both** sides carry a test
asserting their behaviour against the contract, not against their own copy of the
assumption.

**Why.** Three defects of one species reached `main` with every gate green: go-app sent
numeric ids where the store was keyed by registry ids (D-7a); go-app wrote `train_`-prefixed
step names the other side did not know (`996b11d`); and go-app formatted the training cutoff
as RFC3339 while ml-service's parser accepted `YYYY-MM-DD` only, so the ops console's Retrain
button failed on the subprocess's first line of work (D-9). In each case both components were
tested — against their own assumption — and the seam between them was tested by nothing. A
test that says "our default is valid RFC3339" proves nothing about a service that never
accepted RFC3339.

**Where it lives.** `contracts/ops-console.contract.json` is generated from go-app's pipeline
registry (`go test ./internal/services/pipeline -run TestPipelineContract -update`) and is the
one file all three components assert against:

| declared | go-app asserts | ml-service asserts | frontend asserts |
|---|---|---|---|
| `pipeline_steps`, `data_steps`, `lanes` | the registry generates them | — | offers exactly these steps, in this order, with these commands |
| `rejected_query_params`, `rejected_body_params` | `apiparams` generates them | — | constructs none of them |
| `cutoff` (pattern, hint, example) | `DefaultCutoff()` matches the pattern | the CLI parses the example, and go-app's default | the dialog's field asks for `cutoff.hint` |
| `ml_service_calls` (method, path, query) | its call sites use these constants | every path is a route accepting those query parameters | — |
| `format_codes` | generated from `internal/formats` | the same set as `ml.xi.contract.FORMAT_CODES` | — |

**How to add one.** Declare it in Go beside the code that uses it
(`internal/services/pipeline/boundary.go` for the ml-service boundary), add it to
`contractDoc` in `registry_test.go`, regenerate, then add the far side's assertion —
`ml-service/tests/test_ops_console_contract.py` or `frontend/src/utils/opsContract.test.ts`.
A declaration with an assertion on only one side is not H-24; it is the same untested seam
with a JSON file next to it.

**Its companion:** H-23 requires a *gate* to name what varies and what is held fixed. H-24 is
the same discipline for *interfaces*. A boundary also deserves one unskipped seam test that
runs the far side's real parser over the near side's real value — the D-9 case is
`test_retrain_endpoint_accepts_the_cutoff_go_app_sends`. A regression test gated behind
`RUN_E2E=1` is a regression test that does not run.

---

## Checks and coverage gates

**The rule:** local `make check-all` matches CI. Run it in the documented order — frontend →
go-app → ml-service → `frontend-backend-sync-check` — and within a component run the fast
checks (lint, format, typecheck) before the tests. When one fails, fix it and re-run **only**
that step, never the whole loop.

**Per component**

| Component | `check-all` runs | CI runs |
|---|---|---|
| frontend | lint, `format:check`, typecheck, build, test (Vitest, with coverage thresholds) | lint, typecheck, build, test |
| go-app | vet, fmt-check, lint, coverage, coverage-check | vet, fmt-check, tests, coverage-check, `deadcode` |
| ml-service | fmt-check, lint-check, `check-reachability`, coverage, coverage-check | the same, via `make -C ml-service ci` |

**`coverage-check` needs a prior `coverage` run.** It reads `go-app/coverage.out`; running it
against a stale profile reports a number from a different run. `make go-app-check` and
`make ci-go` run the two in order.

**Coverage thresholds live in three places per component and must move together, upward only.**

| Component | Where | Current |
|---|---|---|
| go-app | `go-app/Makefile` (`COV_MIN`), root `Makefile` (`COV_MIN_GO`), `.github/workflows/go-app-ci.yml` | 74 |
| ml-service | `ml-service/Makefile` (`COV_MIN`), root `Makefile` (`COV_MIN_ML`), `.github/workflows/ml-service-ci.yml` | 92 |
| frontend | `frontend/vite.config.ts` (`test.coverage.thresholds`) | lines 80, functions 75, statements 80, branches 79 |

The frontend gate has now been silently off twice, for the same reason in two shapes: first the
threshold keys were set at the top level of `coverage`, where Vitest 2 ignores them, and then a
second `frontend/vitest.config.ts` took precedence over `vite.config.ts` so its `test` block —
thresholds included — was never read. Both are fixed; the duplicate config is deleted. The way
to prove a gate is on is to raise it above the measured figure and watch a run go red, which is
how the second one was found.

When a run passes, raise each threshold to the **floor** of the measured figure — not the
rounded one the report prints. pytest-cov decides `fail_under` on the rounded total but writes
its FAIL line from the exact one, so a threshold of 92 against 91.55 % prints "FAIL Required
test coverage of 92% not reached" and still exits 0: a gate that says FAIL and passes. Never
lower a threshold to make a run green — add tests.

**Pre-commit hooks.** `.githooks/pre-commit` runs gofumpt / golines / golangci-lint (Go), ruff
(Python) and prettier / eslint (frontend) on staged files. Run `make install-hooks` after clone.

---

## Dead-code guardrails

Two checks stop unreachable code accumulating. Both run in CI and are available locally.

| Check                | Command                                 | Runs in                                          |
| -------------------- | --------------------------------------- | ------------------------------------------------ |
| Go, whole-program    | `make -C go-app deadcode`               | `go-app-ci.yml`                                  |
| Python, import graph | `make -C ml-service check-reachability` | `ml-service-ci.yml`, and `make -C ml-service ci` |

**Why the existing linters do not cover this.** `golangci-lint`'s `unused` only reports _unexported_ identifiers within a package, so an exported repository method that nothing calls passes it. On the Python side, `ruff` and `pytest` both stay quiet about a module nothing imports, because the module's own tests keep it "used". That combination is how the repo accumulated roughly 8,000 lines of unreachable code before the repo-wide cleanup removed it, and these two checks are what stop it coming back.

### Go — `deadcode`

Runs `golang.org/x/tools/cmd/deadcode -test ./...` and fails on any output. `-test` means test files count as roots, so a function kept alive only by its own test is _not_ reported — deliberately, since deleting test seams like `db.SetDB` would be wrong. The consequence is a blind spot: production-dead code that has tests still passes. `db.InsertBallEvents` is a known example (the live path is `InsertBallEventsTx`).

Pinned via `DEADCODE_VERSION` in `go-app/Makefile`.

### Python — `scripts/py-reachability.py`

Parses every module under `app/` and `ml/` with `ast`, resolves absolute and relative imports, and walks the graph from two kinds of root. Tests are **not** roots.

- **`MODULE_ENTRYPOINTS`** — `app.main` plus the modules invoked as `python -m ml.<mod>` by `app/training_orchestrator.py` and the Makefiles.
- **`SCRIPT_ENTRYPOINTS`** — modules executed directly rather than imported, e.g. `ml.xi.evaluate` (`make evaluate`) and `ml.xi.parity` (`make xi-parity`). These must be _roots_, not allowlist entries: allowlisting silences the module itself but leaves everything it uniquely imports looking dead.

It fails on three things:

1. a module no entrypoint can reach and that is not in `ALLOWED_UNREACHABLE`
2. an `ALLOWED_UNREACHABLE` entry that is now reachable or gone — so the allowlist cannot rot
3. an `ENTRYPOINTS` module that no longer exists — a stale entrypoint would make live modules look dead

Run it without `--check` to see the full picture, or with `--json` for tooling.

**Adding an entrypoint.** If you add a `python -m ml.something` call site, add it to `ENTRYPOINTS`. Otherwise everything it uniquely imports starts failing the check.

**The allowlist is empty, and that is the healthy state.** `ALLOWED_UNREACHABLE` is a last resort for something unreachable on purpose that is not executed at all. If a module _is_ run, add it to `SCRIPT_ENTRYPOINTS` instead so its imports stay reachable too.

### Version pinning

`golangci-lint` is pinned in two places that must move together: `version:` in `.github/workflows/go-app-ci.yml` and `GOLANGCI_VERSION` in `go-app/Makefile`. Previously the Makefile installed `@latest`, so local lint could disagree with CI.

### Path filters

Workflows are path-filtered, so a change touching only files outside those globs runs **no checks at all**. The root `Makefile` was such a gap — the reason a broken make target (C1-2) sat unnoticed — and is now a trigger path for both the go-app and ml-service workflows.

---

## Container crash and OOM diagnosis

**Goal:** Confirm whether a container is crashing due to out-of-memory (OOM) or something else. The watcher monitors go-api, ml-service and postgres, and collects rich diagnostics.

**Who dies is not who caused it.** go-api and ml-service run without a `mem_limit`, so each auto-sizes its concurrency from the *whole* Docker VM's memory (~80% of it) as if it owned the machine. When their peaks overlap, the VM runs out and the kernel picks a victim by OOM score — and `cricket-postgres`, the fattest steady-state resident, is a prime candidate. Compose therefore sets `oom_score_adj: -500` and a `mem_limit` (`POSTGRES_MEM_LIMIT`, default 3g) on postgres so it is deprioritised as a victim and cannot itself exhaust the VM. All three services run `restart: unless-stopped` so a kill does not leave the stack down.

**1. Exit code (entrypoint)**  
When a process exits, the entrypoint script logs the exit code to stderr. View: `docker logs cric-go-api 2>&1 | tail -20` or `docker logs cric-ml-service 2>&1 | tail -20`. **Exit code 137** = SIGKILL; the kernel often sends this when the OOM killer terminates the process → strongly suggests OOM.

**2. Memory stats in logs (go-api)**  
The go-api logs “memory stats” at startup (`heap_alloc_mb`, `heap_sys_mb`, `heap_inuse_mb`, `sys_mb`, `num_gc`). Set **MEM_STATS_INTERVAL** (e.g. `5m` or `10m`) so the same stats are logged periodically. If OOM-killed, the last log line shows how high memory was. Example: `MEM_STATS_INTERVAL: "5m"` in docker-compose environment. Use `docker logs cric-go-api` to inspect.

**3. Watcher container**  
The stack includes a **watcher** service that monitors **cric-go-api**, **cric-ml-service** and **cricket-postgres** for crashes. On any container exit (die event), it logs:

- **Which service crashed** — clear header, e.g. `CRASH DETECTED: cric-go-api`
- **Diagnostics** — `exit_code`, `OOMKilled`, `image`, `memory_limit_bytes`, `started_at`, `finished_at`, `state_error`
- **Service-specific hints** — e.g. for ml-service: increase `mem_limit`, or fit fewer formats at once
- **Last N log lines** — tail of container logs (default 50) for immediate context

Start: `docker compose up -d`. View: `docker logs -f cric-watcher`. Set `WATCH_LOG=/logs/watcher.log` to persist. Use `WATCH_CONTAINERS=cric-go-api,cric-ml-service,cricket-postgres` (default) or `WATCH_CONTAINER=cric-go-api` (legacy). Use `WATCH_TAIL_LOGS=100` to capture more log lines.

**4. Watcher on host**  
Run the same script on the host: `./scripts/watch-containers.sh`. Options: `WATCH_CONTAINERS=cric-go-api,cric-ml-service,cricket-postgres`, `WATCH_LOG=/path/to/file`, `WATCH_TAIL_LOGS=50`.

**5. After a crash**

- Exit code: `docker inspect cric-go-api --format '{{.State.ExitCode}}'` (or `cric-ml-service`); 137 → likely OOM.
- OOMKilled: `docker inspect cric-go-api --format '{{.State.OOMKilled}}'` (valid for current exited instance; watcher captures at die time).
- Last logs: `docker logs cric-go-api 2>&1 | tail -100` or `docker logs cric-ml-service 2>&1 | tail -100`
- Memory limit: If compose sets `mem_limit`, kernel can kill when usage exceeds it; increase limit or reduce workload.
- **go-api (Import):** import is the only pipeline go-app runs workers for — precompute, export and seqcalc went with their pipelines in P-6. Set **GOMEMLIMIT** or **IMPORT_CONCURRENCY**; with a memory limit detected, workers are sized so total usage stays at about 85 % of it, at 150 MB each. See **config-and-data.md**.
- **ml-service (Retrain):** the rating pass scans the whole event store and the four formats fit concurrently. Increase `mem_limit` (e.g. 4g) or lower **ML_TRAIN_FORMAT_WORKERS**. There are no per-model `n_jobs` settings — `ml-service/config.json` has four keys and no hyperparameters.

**Summary:** Exit code 137, OOMKilled=true, and the watcher's "CRASH DETECTED" output (including which service and last logs) support OOM diagnosis. Use MEM_STATS_INTERVAL for go-api and the watcher for visibility.

## Frontend blank page

A white page in the dev server has two possible causes; read the browser console before changing
anything.

`frontend/index.html` carries a bootstrap guard that paints a diagnostic panel when startup
fails. A module-evaluation failure happens before React mounts, so the React error boundary
(`src/components/common/RootErrorBoundary.tsx`) cannot catch it — the guard covers that gap, and
the boundary covers render-time throws. If you get a truly blank page, the guard itself did not
run.

**1. Stale Vite dependency cache.** The console shows `<name>_default is not a function` (usually
`styled_default`), `Failed to fetch dynamically imported module`, `504 (Outdated Optimize Dep)`,
or `does not provide an export named`. Fix:

```sh
cd frontend && npm run dev:clean
```

Then hard-reload the tab (Cmd+Shift+R) — a normal reload can reuse the stale module graph.

_Why it happens:_ every distinct specifier into a package is its own esbuild pre-bundle entry.
Many entries force the package to be code-split across shared chunks; a re-optimization re-splits
them all, and because Vite serves dep chunks by path while ignoring the `?v=` hash, an already-open
tab receives newly-split files at its old chunk URLs. A lazy `init_` binding then goes missing and
the module throws at evaluation time.

_Why it used to recur after batches of frontend work:_ re-optimization is triggered when Vite
discovers a **new** dependency entry — which is exactly what adding one more `@mui/material/X`
import did.

_Why it should not recur:_ MUI now has exactly one pre-bundle entry (the barrel), enforced by
`no-restricted-imports` in `frontend/.eslintrc.cjs`. Adding a component to an existing barrel
import creates no new entry, so nothing triggers a re-optimization. Check the cache is healthy
with:

```sh
ls frontend/node_modules/.vite/deps/chunk-*.js | wc -l   # ~10 is healthy; ~90 means split
```

**2. A real application error.** Any other console error — a `TypeError` in a component, a bad
import path. `dev:clean` will not help; fix the code.

The `fix-frontend-blank-page` skill walks through this triage.
