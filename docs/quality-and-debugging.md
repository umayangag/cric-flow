# Quality and debugging

This document covers Go unit testing standards, mandatory quality steps (check-all, coverage, CI, hooks, branch protection, PR template), and container crash/OOM diagnosis.

---

## Go unit testing standards

These standards define the gold-standard unit test style for the Go codebase (based on `go-app/internal/services/weatherimport/service_test.go`). All new and refactored tests must follow this guidance.

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

**Canonical skeleton:** One test function with `t.Parallel()`, a table of cases with `name`, inputs, `arrange` (ctx, mocks), `assert` (t, got, err, mocks). Loop with `t.Run(tc.name, ...)`: Arrange → Act (sut.DoSomething) → Assert. Use `t.Helper()` for helpers. Reference: `go-app/internal/services/weatherimport/service_test.go`.

**Naming & organization:** Test files `*_test.go` next to code. Single top-level test per primary behavior; clear case names. Helpers small and pure.

**Assertions:** Use `require.NoError`, `require.Error`, `require.ErrorContains`. Assert return values and mock call counts when side-effects matter.

**Concurrency:** Use `t.Parallel()` at the start of the test function; in subtests only if mocks/fixtures are isolated and no global state is mutated.

**Commands:** `cd go-app && go test -race -cover ./...`; for a package `go test -race -cover ./internal/services/...`; coverage report: `go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out`.

---

## Mandatory steps for system quality

**Goal:** Local `make check-all` matches CI; consistent coverage gates; config changes trigger checks; pre-commit and branch protection in place.

**1. Local vs CI**
- Frontend: check-all runs lint, format:check, typecheck, build, test; CI should include lint + typecheck (done in repo).
- go-app: check-all includes vet, fmt-check, lint, coverage-check; CI runs vet, fmt-check, tests, coverage-check (COV_MIN=30). go-app-check includes coverage-check (done).
- ml-service: lint-check, fmt-check, coverage, coverage-check (80%); CI 80% (OK).

**2. Coverage thresholds**
- go-app: Makefile default COV_MIN_GO=80; CI workflow may use lower COV_MIN (e.g. 30) so CI stays green. Document and raise COV_MIN in `.github/workflows/go-app-tests.yml` as coverage improves.
- ml-service: 80% everywhere (OK).

**3. CI on config changes**
- Path filters may exclude `configs/`, root `Makefile`. Add those paths to relevant workflows or add a lightweight “config change” job (vet/lint for go-app and ml-service).

**4. Pre-commit hooks**
- `.githooks/pre-commit` runs gofumpt/golines/golangci-lint (Go), ruff (Python), prettier/eslint (frontend) on staged files. Run `make install-hooks` after clone.

**5. Branch protection**
- Require status checks before merge: e.g. Go App Lint, Go App Tests, ML Service Tests, Frontend Tests. Optional: aggregated “check-all” workflow.

**6. PR template**
- `.github/PULL_REQUEST_TEMPLATE.md` includes Go unit test standards checklist and reminders to run go-app and ml-service tests. Optional: add “Ran `make check-all` (or component checks) and all passed.”

**Checklist:** go-app-check includes coverage-check (done); frontend CI runs lint and typecheck (done); go-app CI COV_MIN set and documented (pending); path filters or config job (pending); branch protection (pending); PR template “make check-all” checkbox (optional).

---

## Container crash and OOM diagnosis

**Goal:** Confirm whether go-app (go-api) container is crashing due to out-of-memory (OOM) or something else.

**1. Exit code (entrypoint)**  
When the API process exits, the entrypoint script logs the exit code to stderr. View: `docker logs cric-go-api 2>&1 | tail -20`. **Exit code 137** = SIGKILL; the kernel often sends this when the OOM killer terminates the process → strongly suggests OOM.

**2. Memory stats in logs**  
The API logs “memory stats” at startup (`heap_alloc_mb`, `heap_sys_mb`, `heap_inuse_mb`, `sys_mb`, `num_gc`). Set **MEM_STATS_INTERVAL** (e.g. `5m` or `10m`) so the same stats are logged periodically. If OOM-killed, the last log line shows how high memory was. Example: `MEM_STATS_INTERVAL: "5m"` in docker-compose environment. Use `docker logs cric-go-api` to inspect.

**3. Watcher container**  
The stack includes a **watcher** service that logs container exit code and OOM status via the Docker socket. Start: `docker compose up -d`. View: `docker logs -f cric-watcher`. When go-api exits, the watcher logs `exit_code` and `OOMKilled`. **137** or **OOMKilled=true** → likely OOM. Optional: `WATCH_CONTAINER=cric-ml-service`; `WATCH_LOG=/logs/watcher.log` to persist. Watcher: `Dockerfile.watcher`, `scripts/watch-containers.sh`; needs Docker socket in compose.

**4. Watcher on host**  
Run the same script on the host: `./scripts/watch-containers.sh`. Options: `WATCH_CONTAINER=cric-go-api`, `WATCH_LOG=/path/to/file`.

**5. After a crash**
- Exit code: `docker inspect cric-go-api --format '{{.State.ExitCode}}'` (137 → likely OOM; if restarted, use watcher or events for last run).
- OOMKilled: `docker inspect cric-go-api --format '{{.State.OOMKilled}}'` (valid for current exited instance; watcher captures at die time).
- Last logs: `docker logs cric-go-api 2>&1 | tail -100` for “exited with code …” and last “memory stats”.
- Memory limit: If compose sets `mem_limit`, kernel can kill when usage exceeds it; increase limit or reduce workload.

**Summary:** Exit code 137, OOMKilled=true, high last “memory stats”, and entrypoint “exited 137” in logs all support OOM. Use MEM_STATS_INTERVAL and the watcher container (or host script) for visibility.
