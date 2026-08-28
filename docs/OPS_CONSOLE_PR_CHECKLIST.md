# Ops Console: run the whole pipeline from the frontend

**Goal.** Acquire a new Cricsheet dataset, extract it, preprocess it and train on it
without opening a terminal — with enough visibility while it runs that the terminal is
not missed.

**Scope decisions** (agreed before planning):

| Decision | Choice | Consequence |
|---|---|---|
| Data source | **Fetch from a Cricsheet URL** | Backend downloads and unzips. No browser upload path, no host-path picker. Server needs outbound network |
| Deployment | **Localhost now, remote later** | Keep the API-key middleware; but no host paths in the UI, no unbounded downloads, and SSRF/zip-slip defences land now rather than as a retrofit |
| Visibility | **Structured milestones + metrics** | Typed events (`fold 3/5`, `RMSE 24.1`), not raw stdout. Needs each trainer instrumented; renders as real progress rather than a log tail |

---

## What already exists

Worth stating plainly, because it is more than it looks and it changes what this plan
has to build. Verified against `main`, not assumed:

- **11 steps are already API-triggerable** — `POST /ops/pipeline/run/{step}` covers
  `import`, `precompute`, `export`, the six `train_*`, `train_combination_meta` and
  `auto_tune`.
- **Order is enforced server-side.** `CanRunPipelineStep` refuses a step whose
  predecessor has not completed successfully, and refuses a step already running.
- **A global busy-lock exists** — `HasPipelineBusy` — so two steps cannot overlap.
- **Cancellation works**: `POST /ops/pipeline/stop`, wired to a per-job
  `context.CancelFunc`.
- **Progress streams over SSE** at `GET /ops/pipeline/stream`.
- **Run history is persisted** in `data_migrations` (`command`, `args`, `started_at`,
  `completed_at`, `status`, `metadata` jsonb, `error_message`) and exposed at
  `/ops/migrations`.
- **The UI is substantial already**: `OpsPipelineGraph`, `PipelineProgressPanel`,
  `PipelineStepDialog`, `OpsMatrix`, `OpsSuggestions`, `HealthTab`.

So this is not a greenfield build. It is three specific gaps.

---

## The three real gaps

### 1. Acquisition and extraction do not exist at all

`importCricSheetHandler` reads a **server-side directory**, defaulting to `../data`.
There is no download, no upload, no unzip anywhere in `go-app` — verified by searching
for `archive/zip`, `multipart`, `FormFile` and outbound `http.Get`: no hits.

**This is the one step that genuinely cannot be done from the frontend today.** Getting
a new dataset onto the box is a terminal task by construction.

### 2. Log visibility is structurally blocked, not merely unbuilt

`training_orchestrator.run_training_subprocess` calls:

```python
proc = subprocess.run(cmd, cwd=root, env=env, capture_output=True, text=True, timeout=timeout_sec)
```

`capture_output=True` buffers everything until exit. On success the output is
**discarded**; on failure only a **tail** is logged. During a ten-minute training run
there is nothing to observe, anywhere, at any fidelity.

No amount of frontend work fixes this. The subprocess invocation has to change.

### 3. Progress is step-level, single-slot, and manual

- Only `precompute` reports real progress (formats, phase, ETA). Every other step shows
  a label and an elapsed counter.
- The SSE handler reports `inProgress[0]` — **one** running step, even though the
  history table can hold several.
- Nothing chains. `make up-all` and `make full-pipeline` have no API equivalent, so a
  full run is eleven manual clicks with waiting in between.

---

## The pattern to build on

`auto_tune` already solved the hard part of cross-process progress, and it is worth
copying rather than reinventing:

- `ml/auto_tune_progress.py` writes progress to a JSON file.
- `training_orchestrator.get_auto_tune_progress()` reads that file.
- `GET /admin/train/auto-tune/progress` serves it.

A subprocess cannot push into its parent's memory, but it can write a file. Generalising
this one module into a shared progress channel used by every trainer is the smallest
change that delivers the agreed fidelity, and it keeps one mechanism instead of two.

---

## Status

| ID | Status | Branch / PR | Summary |
|----|--------|-------------|---------|
| F-1 | done | `ops/pr1-ui-honesty` | `train_combination_meta` missing from the frontend step list |
| F-2 | done | `ops/pr2-multi-step-progress` | SSE reports only one in-flight step |
| F-3 | done | `ops/pr3-dataset-directory` | Make the dataset directory a first-class, observable thing |
| A-1 | done | `ops/pr4-data-fetch` | `POST /ops/data/fetch` — download a Cricsheet archive |
| A-2 | done | `ops/pr5-data-extract` | `POST /ops/data/extract` — unzip into the data directory |
| A-3 | done | `ops/pr6-dataset-registry` | Dataset registry: what is on disk and where it came from |
| A-4 | done | `ops/pr7-data-tab` | Frontend: Data tab — feeds, fetch, extract, registry |
| O-1 | done | `ops/pr8-progress-channel` | Generalise `auto_tune_progress` into a shared progress channel |
| O-2 | done | `ops/pr9-instrument-trainers` | Instrument the six trainers to emit milestones and metrics |
| O-3 | done | `ops/pr10-serve-progress` | Serve generalised progress; fold into the existing SSE stream |
| O-4 | done | `ops/pr11-persist-metrics` | Persist final metrics to `data_migrations.metadata` |
| O-5 | done | `ops/pr12-run-history` | Frontend: per-step progress, metrics, run-history drill-down |
| R-1 | done | `ops/pr13-run-plan-executor` | Server-side run-plan executor (chaining, stop-on-failure, resume) |
| R-2 | done | `ops/pr13-run-plan-executor` | `POST /ops/pipeline/run-plan` and plan status |
| R-3 | done | `ops/pr14-run-plan-ui` | Frontend: "Run full pipeline" with per-step live state |
| P-1 | done | `ops/pr15-export-provenance` | Stamp dataset provenance into exports and model sidecars |
| P-2 | done | `ops/pr16-workbench-provenance` | Show which dataset produced which model |

**Recommended order:** F → A → O → R → P. F is trivial and makes the UI honest. A
removes the only impossible-from-UI step. O makes long runs tolerable to watch. R is
convenience on top of a working system. P is the payoff that makes results explainable.

---

## Phase F — Foundation

Small, and each makes something currently misleading correct.

### F-1 · `train_combination_meta` missing from the frontend step list

**Why:** C5-2 implemented the backend step and the go-app handler, but
`frontend/src/utils/pipelineSteps.ts` was never extended. The backend accepts the step;
the UI does not offer it. This is a gap introduced by the cleanup work, not a pre-existing one.

- [x] Add the step to `pipelineSteps.ts` after `train_win`/`train_innings`
- [x] Mark it optional — it needs `backtest_contributions.csv`, and returns
      400 `CONTRIBUTIONS_CSV_MISSING` without it
- [x] Surface that 400 as an actionable message naming the producing action, not a red toast
      (`Step.Prerequisite` in the registry, shown in `PipelineStepDialog` before the run;
      `pipelinesvc.MLError` keeps ml-service's `{code, message, hint}` intact in
      `data_migrations.error_message` instead of flattening it)
- [x] Reflect it in `OpsPipelineGraph` (the graph renders `derivePipelineSteps`, so
      adding the step there is the whole change)

**Done by:** a single step registry (`go-app/internal/services/pipeline/registry.go`) that
replaces the six parallel step tables — two switches in the run handler, three maps in
`services/pipeline`, two in `opsstatus`. `contracts/ops-console.contract.json` is generated
from it and asserted from both sides.

**Acceptance:** every step the backend accepts appears in the UI, and vice versa.
**Verify:** a test asserting the UI step-id list matches the backend's accepted set —
otherwise this drifts again.

### F-2 · SSE reports only one in-flight step

**Why:** `pipelineProgressStreamHandler` takes `inProgress[0]`. With the current global
busy-lock that is *usually* true, but it is an assumption baked into the transport,
and R-1 (chaining) will make it false.

- [x] Change the payload to carry a list of running steps, not a single one —
      `{running, steps: [...]}`, most recently started first
- [x] Migrate the panel in the same PR rather than keeping a compatible shape. A
      mirrored `step_id` alongside `steps[0]` would be a second way to say the same
      thing, and the only consumer is our own ops console
- [x] Decided: **no**, `fetch`/`extract` do not share the training lock. Encoded as
      `Step.Lane` in the registry (`LaneCompute` / `LaneData`): steps in a lane run
      one at a time, lanes overlap. Phase A's steps declare `LaneData` and nothing
      else changes

**Also fixed here:** `pipeline.PipelineCommands` — a *seventh* copy of the step list,
and the one the busy-lock actually consulted — was missing `train-combination-meta`,
so that step could overlap a training run despite the lock existing to prevent exactly
that. The lane now comes from the registry, and `TestLaneBusyCoversEveryComputeStep`
is the regression guard.

**Refactor:** `pipelineProgressStreamHandler` was a 90-line closure mixing SSE
transport, payload assembly and ETA arithmetic. It is now `sseStream` (transport),
`progressReporter` (payload, with injectable clock and status sources so the ETA is
testable without a live stream), and on the frontend `usePipelineProgressStream`
(connection and retry) plus `PipelineStepProgressCard` (one step's display).

### F-3 · Make the dataset directory a first-class, observable thing

**Why:** the data location is a defaulted string (`"../data"`) buried in a handler.
Nothing reports what is in it. Acquisition needs this to exist first.

- [x] Promote the data directory to config, with an env override — one resolver,
      `dataset.Dir()`: `GO_APP_CRICSHEET_DIR`, then `inputs.cricsheet_dir`, then the
      built-in default. The CLI, the import handler and `/ops/status` all use it
- [x] Add it to `/ops/status`: path, exists, match-file count, total bytes, newest file
      and its mtime, plus the env var name so the UI can say how to change it
- [x] Show it in the Ops UI (`OpsDatasetSection`), calling out missing and empty
      directories rather than showing a zero and leaving you to notice

**Acceptance:** you can tell from the browser whether there is any data on the box.

**The bug this uncovered.** The location was a defaulted string in the handler
(`"../data"`), a *different* default in config (`"../data/go-app/cricsheet"`), and an
env var only the CLI read. `cricsheet.ImportDir` does not recurse, so an import
started from the ops console read the wrong directory, found no `.json` files, and
**completed successfully with zero rows** — the third instance of this repo's
silent-success failure mode.

Two fixes, both here: every caller resolves the directory the same way, and
`ImportDir` now verifies its own postcondition — zero match files is
`ErrNoMatchFiles`, naming the directory it looked in and the env var that moves it.
`dataset.MatchFiles` is shared by the importer and the status section, so the count
the browser shows is exactly the set import will read.

---

## Phase A — Acquisition

The part that removes the terminal from the loop.

### A-1 · `POST /ops/data/fetch`

Download a Cricsheet archive to a staging directory as a tracked background job.

- [x] Background job via `pipeline.RunJob`, like every other step — returns 202, not a
      held-open request. A large archive over a slow link must not be request-scoped
- [x] Named feeds (`all`, `t20s`, `odis`, `tests`, `ipl`) resolved server-side to URLs,
      plus an optional explicit URL — both go through the same allowlist check
- [x] Report bytes-downloaded / total / rate through the progress channel — folded into
      the existing `/ops/pipeline/stream` payload as `steps[].fetch`, not a second stream
- [x] Write to `<cricsheet_dir>/_staging/`, never directly into the live data directory.
      A subdirectory rather than a sibling: one env var still moves everything, the
      extract-and-swap A-2 needs stays on one filesystem, and `ImportDir` cannot see it
- [x] Record source URL, HTTP `ETag`/`Last-Modified`, size and SHA-256 in job metadata,
      and in a `.meta.json` sidecar beside the archive so the ETag survives a restart

**Defences that land now, not later** — because "remote later" was the answer:

| Risk | Mitigation |
|---|---|
| **SSRF** — arbitrary server-side fetch | Host **allowlist** (`cricsheet.org` and its CDN). Reject redirects that leave it. Never fetch a bare user-supplied host |
| **Disk exhaustion** | Check free space before starting; hard cap on `Content-Length`; abort and clean up staging on overrun |
| **Silent truncation** | Verify the byte count and record the digest. A short read must fail loudly — this is exactly the `--fail`-less-curl bug from C1-2, in new clothes |
| **Wasted re-download** | Send `If-None-Match`; treat 304 as success with "already current" |

**Done by:** `go-app/internal/services/dataacquire`. Every row above is a test with a
crafted case, not an assertion in a comment: `TestResolveSource_RejectsOffAllowlistHosts`
covers the metadata service, loopback, private ranges and suffix lookalikes;
`TestNewClient_RejectsRedirectsOffTheAllowlist` covers the 302 that defeats a check done
only on the typed URL; the cap is tested both by declared `Content-Length` and by
outgrowing it mid-stream on a chunked response.

**Two structural changes this needed.**

`Step.Surface` — the registry is the one list of steps (F-1), but `fetch` is not a stage
of the pipeline and must not appear on its graph. `SurfaceData` keeps its lane, label and
busy-check derived from the same place as every other step while keeping it off the
ordering; `/ops/pipeline/run/fetch` refuses with a pointer to `/ops/data/fetch` rather
than a bare 400. The contract gained a `data_steps` list alongside `pipeline_steps`.

**The bug this uncovered.** F-2 gave acquisition its own lane so a download would not
block training. Two things still assumed one global job: `App.currentJobCancel` was a
single `context.CancelFunc`, and `tracking.CancelInProgressMigration` cancelled
`inProgress[0]` — the same single-slot assumption F-2 fixed in the stream, in two places
it did not reach. The moment a fetch and a training run actually overlapped, starting the
fetch would overwrite the training job's cancel func, making it uncancellable, and Stop
would kill one job while marking a *different* one CANCELLED — one run dead but recorded
as running, another recorded as cancelled but still going.

Both are now keyed by lane (`App.SetJobCancel`/`CancelJobsInLanes`,
`tracking.CancelInProgressMigrations`), and `StopRun` scopes both halves to the *same*
lane set so they cannot disagree. `POST /ops/pipeline/stop` takes an optional `?lane=`
and still stops everything without one. `TestJobCancels_AreKeyedByLane` and
`TestStopRun_ScopesBothHalvesToTheSameLane` are the regression guards.

### A-2 · `POST /ops/data/extract`

- [x] Unzip from staging into the data directory as a tracked job with entry-count
      progress, folded into the same SSE stream as `fetch`
- [x] **Zip-slip defence.** Traversal segments, absolute paths, backslash-separated
      traversals (normalised *before* the check, not after), `C:\` drive prefixes,
      `//host/share` UNC paths, and symlink or other irregular entries. The resolved
      path is then re-checked against the destination root *with a trailing separator*,
      so `/data/cricsheet-evil` cannot pass as a prefix match of `/data/cricsheet`
- [x] Cap entry count, total uncompressed bytes and per-entry expansion ratio —
      checked up front from the central directory **and again mid-inflate**, because
      the declared sizes are attacker-controlled and a bomb can understate itself
- [x] Nothing enters the dataset directory until the whole archive has been inflated
      and checked. See the note below on why this is not a single directory rename
- [x] Write a manifest (`.dataset-manifest`): entry count, bytes, source archive digest
      and URL, completion time. A-3's registry and P-1's provenance stamp read from it
- [x] Also: refuse an archive that extracts without producing a single match file.
      Extracting "successfully" into an empty dataset is this repo's recurring failure
- [x] `GET /ops/data/staged` — the archives available to extract, each with the
      provenance its fetch recorded, plus the manifest of the dataset currently live

**Acceptance:** a malicious archive (traversal entry, symlink, bomb) is rejected with a
clear error and leaves no files outside the destination. Write these as tests with
crafted fixtures — do not assume the stdlib refuses on your behalf, `archive/zip` does not.

**Done, and verified the hard way.** Every refusal above has a crafted fixture in
`extract_test.go`, built by writing zip headers directly rather than through the
`Create` helper — the entries a well-behaved writer would never emit are the whole
point. The suite was then re-run with the path, symlink and mode checks removed: all
fourteen cases fail without them. A security test that has never been seen to fail is
not evidence.

**Not a temporary sibling, and why.** The plan called for extracting to a sibling and
swapping by rename. That is not available here: the staging directory lives *inside*
the dataset directory (F-3's decision, so one env var moves everything and the swap
stays on one filesystem), so renaming the dataset directory away would take the archive
being read with it. What is done instead gives the same guarantee for the risk that
matters — the entire archive is inflated and checked inside staging, and only then are
already-verified files moved across with same-filesystem renames. Extraction replaces
rather than merges; the displaced contents go to `_staging/previous-<timestamp>/`, so a
mistaken replace is recoverable.

### A-3 · Dataset registry

- [x] Persist one row per acquired dataset: feed, source URL, digest, fetched-at,
      extracted-at, entry count, match-file count, bytes, destination
      (migration `0002_datasets.sql`, table `datasets`)
- [x] `GET /ops/data/datasets`, newest first, bounded limit
- [x] Mark which dataset is currently live in the data directory

**Why it matters:** without this, P-1/P-2 have nothing to reference, and
"which data produced this model?" stays unanswerable.

**The digest is the identity, not the filename.** Cricsheet reuses filenames across
releases — `all_json.zip` is always `all_json.zip` — so a filename-keyed registry would
conflate every dataset ever fetched into one row. Both steps upsert on `sha256`: a
re-fetch of unchanged bytes updates the existing row instead of duplicating it, and
leaves the extract columns untouched, because re-downloading an archive does not
un-extract it. Extract can *insert*, not only update, because an archive can reach
staging without passing through fetch.

**No `is_live` column, deliberately.** Which dataset is live is a property of the
filesystem — an operator who rsyncs files into the data directory changes it without
touching Postgres, and a stored flag would go on asserting the old answer. It is derived
by matching the on-disk `.dataset-manifest` digest against `sha256`, so it cannot go
stale. The endpoint returns `live_sha256` separately, so "the box holds a dataset this
registry has never seen" is visible rather than rendered as an empty list.

**Extract now hashes an archive that has no sidecar**, at the cost of one extra read of
a local file. A hand-placed archive would otherwise land in the registry with a blank
digest — and a dataset with no digest is exactly the case P-2 exists to flag, so leaving
it blank would defeat the point of recording it.

**Unknown is not zero.** Every optional column is nullable and scanned through
`sql.Null*`: a hand-placed archive has no feed or URL, and a downloaded-but-not-extracted
one has no entry count. "We do not know where this came from" and "it came from nowhere"
are different answers, and the second is never true.

**A registry write failure is logged, not fatal.** The step has already succeeded — the
bytes are on disk — and failing the job would report work that happened as work that did
not. Provenance survives regardless: it is in the run's `data_migrations` metadata, the
archive's sidecar and the extracted directory's manifest. The registry is the index over
those, not their only copy.

**Verified against Postgres, not only mocks.** The unit tests use the DB mock, which
proves the queries are *sent* but not that Postgres accepts them — an `ON CONFLICT`
naming a non-unique column would pass all of them and fail on the box. Seven integration
tests in `registry_integration_test.go` run the real SQL (`RUN_DB_TESTS=1`), covering the
re-fetch-does-not-erase-the-extract case, provenance preservation, the hand-placed
archive, live marking and ordering. They were run locally against a disposable
`postgres:15-alpine` container; CI applies migrations but does not yet set
`RUN_DB_TESTS=1`.

### A-4 · Frontend: Data tab

- [x] Feed picker plus optional URL, with the allowlist rule stated in the UI
- [x] Live fetch progress (bytes, rate, ETA) and extract progress (entries)
- [x] Registry table with the live dataset marked
- [x] "Fetch → Extract → Import" offered as a sequence once R-1 exists — **done, and
      not here.** R-1 landed, and the sequence went server-side as the `import` run
      plan (consumer plan W6-2): Import downloads, extracts and loads, skipping what
      the box already has and saying why. The reasoning above held — chaining in the
      browser would have been a second executor that loses the run on a page reload.

> **Superseded.** This tab no longer exists. Once Import acquired its own data there
> was nothing left on it to decide, and the one part that was never an action — the
> dataset registry — moved to Ops Status beneath the directory it describes. See
> [CONSUMER_SURFACES_PR_CHECKLIST.md](CONSUMER_SURFACES_PR_CHECKLIST.md) W6-3. The
> `/ops/data/*` endpoints A-1 to A-3 built are unchanged and still serve the plan.

**Its own tab, not a section of Ops Status.** Acquisition is what you do *before* the
pipeline, not a stage of it — the same distinction `Step.Surface` encodes on the
backend, which is why the pipeline graph does not list fetch and extract.

**Feed and URL are a choice, not two boxes.** The backend refuses both at once as an
ambiguity rather than resolving it, so the UI offers a toggle. Two fields that can
silently disagree would make a 400 the operator's problem to decode.

**Refusals carry what would be accepted.** `postDataJob` returns status alongside body
rather than throwing on non-2xx, because the useful half of a 400 is the body:
`allowed_hosts` is what turns "rejected" into "here is what is allowed", and the
staged-archive list is what turns "nothing staged" into a next step.

**Live progress reuses the one SSE stream** and `PipelineProgressPanel`. `fetch` and
`extract` render inside `PipelineStepProgressCard` alongside precompute and auto-tune,
so there is one renderer, not a second progress mechanism to keep in sync.

**Unknown is not zero, in three places.** A step that has started but not published a
sample says "Starting…" rather than "0 B" (which reads as a stall); a download with no
`Content-Length` gets an indeterminate bar rather than an invented percentage; and a
never-extracted dataset shows an em dash for match files, not `0`.

**The busy state is asked of `/ops/status`,** not inferred from "we just clicked". A
step may have been started from another browser tab or left over from a previous
session, and a button that looks available but 409s is worse than one that says why it
is disabled.

**A live dataset the registry has never seen is called out.** Because `live` is derived
from the directory's manifest rather than stored, `live_sha256` can match no row — a
directory populated before the registry existed, or by hand. Rendering that as a table
with nothing highlighted would read as "nothing is live", which is a different and
wrong answer.

---

## Phase O — Observability

Agreed fidelity: **structured milestones and metrics**, not raw stdout.

### O-1 · Generalise the progress channel

**Why:** `ml/auto_tune_progress.py` already does file-backed cross-process progress
correctly. Generalise it once rather than growing a second mechanism.

- [x] Extract a shared module: `ml/run_progress.py` — `configure`, `set_progress_file`,
      `emit(event)`, `clear`, plus the reader half (`read`, `latest_for_step`) so the
      staleness rule lives in one place rather than in each endpoint
- [x] One progress file **per run**, not one global file. Named
      `<step>__<run_id>.json`; `run_id` comes from `PIPELINE_RUN_ID` or the pid, and is
      sanitised into the filename — it arrives from go-app and from env, so it chooses
      no paths of its own
- [x] Typed event schema, versioned from the start:

```json
{ "v": 1, "run_id": "...", "step": "train_batting", "phase": "cv",
  "current": 3, "total": 5, "metrics": {"rmse": 24.1}, "ts": "..." }
```

- [x] Keep `auto_tune` working on the generalised module — do not leave two paths.
      `ml/auto_tune_progress.py` is now a thin adapter: it keeps its dozen tuning-specific
      keyword arguments, because forcing those through the shared `Event` at every call
      site would make the emitters harder to read, but it owns no mechanism
- [x] Writes must be atomic (temp file + rename) so a reader never sees a partial JSON.
      The temp file is created in the destination directory: `os.replace` is only atomic
      within one filesystem, and `/tmp` is often a different one

**The compatibility constraint, and how it is met.** `GET /admin/train/auto-tune/progress`
is polled by go-app and rendered by the ops console, which read `phase`, `algorithm`,
`trial`, `best_score` and friends at the top level. The envelope (`v`, `run_id`, `step`,
`ts`) was added *underneath* those fields rather than in place of them, so nothing
downstream changes. `test_keeps_the_payload_shape_go_app_reads` asserts that field by
field — if it ever stops being true the console silently loses its auto-tune detail,
which is not a failure that announces itself.

**Two things the generalisation forced, both improvements.** With per-run files there is
no single path to read, so `get_auto_tune_progress` now asks for *the live run*: the
newest non-stale file for the step. That closes a hole the fixed path had — a crashed
run left its file behind and the endpoint reported it as current, showing a run that was
not happening. `AUTO_TUNE_PROGRESS_FILE` still pins a path for tests and for an operator
who wants to `tail` one file.

**Emission never breaks a training run.** Every failure — unwritable directory, full
disk, an observer that raises — is logged and swallowed. Progress is telemetry; a
ten-minute training run that succeeded must not be reported as failed because the
progress file could not be written.

### O-2 · Instrument the six trainers

- [x] `train_batting`, `train_bowling`, `train_fielding`, `train_extras`, `train_win`,
      `train_innings` emit: data loaded (rows, columns, targets), fitting started,
      artifact written (name, bytes), format finished
- [x] CV fold progress and per-fold metrics — **only where cross-validation exists.**
      `train_win` splits with `TimeSeriesSplit` and now emits accuracy, Brier and log
      loss per fold; the others fit once. A step emitting fake folds to look busy would
      be worse than one that says nothing
- [x] Also emit the **dropped low-variance columns** — that is where a silently constant
      feature like `weather_composite` becomes visible instead of being quietly discarded
- [x] Emission must never break training: wrap in try/except, log and continue

**Done by `ml/training_progress.py`** — a milestone vocabulary over O-1's transport.
Named milestones rather than a dict to fill in: every trainer emitting the same names is
what lets one panel render all of them, and what stops "rows" being `rows` in one step
and `n_rows` in the next.

**Ten drop sites, all of them discarded.** Every trainer already called
`drop_low_variance_columns`; every one assigned the result to `_dropped` and moved on.
A feature that had gone silently constant — an export bug, a column the pipeline stopped
populating — was removed without anyone being told. All ten now report.
`test_every_low_variance_drop_is_reported` is a source check rather than a behavioural
one, deliberately: reaching every site behaviourally needs a valid row schema for four
different trainers, and a test that stubs its way there passes vacuously when the
fixture fails to build. What needs guarding is that no *new* drop site appears without
an emit beside it. Verified by deleting an emit and watching it fail.

**Guarding the transport was not enough.** The first version wrapped only
`run_progress.emit`, and the tests immediately found the hole: what reaches a caller is
usually a failure in the *arguments* — statting a path that turned out to be `None`,
coercing a row count that arrived as a string. Every emitter is now `@never_raises`.

**Completed formats, not "the current format".** Formats train concurrently
(`ML_TRAIN_FORMAT_WORKERS`), so "which format is running" has no single answer and
`current` cannot mean "the one in flight". Counting *completed* formats stays true under
concurrency; the format each event is about travels in `extra`. Asserted with twenty
threads.

**Metrics are cleaned before they travel.** Scores arrive as numpy scalars, and a NaN
from a degenerate fold serialises to the literal `NaN` — not valid JSON, and enough to
make the whole progress file unreadable to a strict parser.

**Note:** `run_training_subprocess` keeps `capture_output=True`. Raw output stays out of
scope by the fidelity decision — the events carry the signal. Revisit only if the
milestones prove insufficient in practice.

### O-3 · Serve and stream the progress

- [x] `GET /admin/train/progress?step=&run_id=`. With no `run_id` it reports the *live*
      run — the newest non-stale file for that step. Naming a `run_id` reads exactly
      that run, finished or stale, which is what a caller asking about a specific run
      wants as opposed to one asking "what is happening now"
- [x] go-app polls it while a training step is in flight and folds the events into the
      existing `/ops/pipeline/stream` SSE payload — **one** stream for the UI, not two
- [x] Handle the ml-service-unreachable case as *unknown*, not as failure

**`/admin/train/auto-tune/progress` survives as a delegate.** It is a released endpoint,
so removing it is a breaking change outside this item's scope — but it has no
implementation of its own. One implementation, two routes.

**"Unreachable" and "nothing yet" are different answers.** `FetchStepProgress` returns
`(nil, nil)` for a step that has published nothing and `(nil, ErrProgressUnavailable)`
when ml-service could not be asked. Collapsing them — which the old
`FetchAutoTuneProgress` did, returning nil for both — renders identically as a blank
panel, but one is a run about to report and the other is a broken link the operator can
act on. The payload carries `progress_unavailable` and the card says so in words,
including that *the step is still running*: saying "failed" about a healthy run whose
telemetry link is down would be worse than saying nothing.

**Only ml-service steps are polled.** `import`, `precompute` and `export` run inside
go-app and have no progress endpoint to ask, so asking about them would be a request per
SSE tick for an answer that cannot exist. The registry's `RunsOnMLService()` is the gate.

**The ETA measures from the run's start, not the last event.** The progress file records
when the last event was written, which says nothing about when the run began — that
comes from the `data_migrations` row. Nothing is reported until at least one unit has
finished: with zero completed there is no observed rate, and a number invented from a
default is one the operator has no reason to believe.

**`FetchAutoTuneProgress` keeps its nil-on-error contract** because its callers cannot
act on the difference; it now delegates to the tri-state fetcher rather than duplicating
the HTTP call.

### O-4 · Persist final metrics

- [x] On completion, write the final metrics into `data_migrations.metadata` (already
      `jsonb`, already the run-history table — no new table needed)
- [x] Include the dataset digest from A-3 to close the provenance loop
- [x] Extend `/ops/migrations` to return it — it already did; what was missing was
      anything worth returning. The frontend `metadata` field is now typed
      (`RunMetadata`) instead of `unknown`

**A result is not progress, and the difference is a lifetime.** The obvious design —
have go-app read the progress file when the run ends — cannot work: `finish()` removes
that file, because one left behind reads as a run still going. So the outcome is written
to a separate `.result.json` beside it, which *outlives* the run, and ml-service returns
it on the training endpoint's response. That response is the only moment the outcome is
still available to go-app.

The two files both end in `.json`, so `list_for_step` excludes results explicitly —
otherwise a finished run would be read as a running one, which is the exact confusion the
two lifetimes exist to avoid.

**The summary is accumulated as events go past.** The progress file holds only the
*last* event; it is overwritten on every emit, which is what makes it cheap to poll. The
run's outcome is a different question — which formats trained, on how many rows, what was
dropped, what was written — and building it as the run proceeds is the only chance to
have it, since nothing re-reads a stream of overwritten files.

**The dataset digest is stamped by go-app, not ml-service.** ml-service does not know
which dataset produced the CSVs it trained on; go-app does, from the manifest A-2 leaves
in the dataset directory. Recording it on the run is what closes the loop this plan
opened by saying nothing could answer *"which data produced this model?"*.

**Provenance that cannot be established is omitted, not blanked.** A dataset directory
populated by hand has no manifest and a hand-placed archive has a digest but no feed or
URL. Absent means genuinely unknown — which is precisely the case P-2 exists to flag, and
inventing a value would defeat it.

**A bug the tests caught.** `format_done` only recorded a format into the summary when it
carried metrics, so a format that trained successfully but published no numbers was
absent from the summary entirely — which reads as "it did not run". Now recorded either
way.

**Losing the summary is not losing the run.** An unreadable success body is logged and
the run still completes: a 200 is a success whatever the body says, and turning it into
an error would report work that happened as work that did not.

### O-5 · Frontend

- [x] Real per-step progress bars driven by `current`/`total` — **landed in O-3**, which
      had to render the payload it introduced. Noted here rather than claimed twice
- [x] Metrics panel per run; highlight change against the previous run of the same step
- [x] Run-history drill-down: args, metrics, error, dataset used. The dialog rendered
      `JSON.stringify({args, meta})`; it now renders the run, with the raw JSON still a
      click away for anything the panel does not model
- [x] Keep the failure message actionable — go-app already formats an ml-service
      precondition as `CODE: message — hint` (F-1's `MLError`), so the string was
      already actionable. What changed is presentation: the code is a chip and the hint
      is its own **Next:** block, instead of being buried mid-way through a red
      monospace paragraph

**Direction is the judgement in this item, and I got it wrong first.** Whether a metric
moving up is good news depends entirely on which metric it is. `train_win` emits
`cv_accuracy_std` — which contains "accuracy", and a naive substring match read it as
higher-is-better, i.e. **reported a model that had got less consistent as an
improvement.** Spread measures are now checked before everything else. The test that
caught it is `treats a spread measure as lower-is-better even when it names a good
metric`.

A metric matching no rule is shown with its delta and **no verdict**: "it changed by
this much" is true regardless, while "this is an improvement" would be a guess.

**Comparison needs a window the table does not have.** The previous run of a step is
usually not on the page being viewed, so opening the dialog fetches one bounded window
(100 rows) and finds the most recent earlier *completed* run of the same command that
recorded metrics. A failed run's numbers are not a baseline. Finding none is reported as
"no earlier run of this step to compare against" — and, until the lookup returns, nothing
is claimed either way.

**Per-format metrics are prefixed, not merged.** `T20I.rmse` and `ODI.rmse` are different
numbers; merging them would silently keep whichever came last.

**Unknown provenance is stated.** A run from before the dataset registry, or against a
directory populated by hand, says so — which is the case P-2 exists to flag.

---

## Phase R — Run orchestration

### R-1 · Run-plan executor

- [x] Server-side sequential executor over an ordered step list (`services/runplan`)
- [x] Stop on first failure, leaving the plan resumable from the failed step. The
      remaining steps stay `PENDING` rather than being marked failed — that is what
      `FirstIncomplete` reads to answer "where would a resume start?"
- [x] Reuse `CanRunPipelineStep` for ordering rather than duplicating the rules. It is
      injected as `Executor.Gate`, so the plan and the single-step endpoint cannot
      disagree about whether a step may run
- [x] Persist plan state so a page reload — or a browser closed overnight — does not
      lose the run. Written to `data_migrations` before and after every step, not only
      at the end, so a reader mid-run can see which step is in flight
- [x] Cancellation must stop the plan, not just the current step

**R-1 and R-2 landed together, and could not sensibly be split.** `make deadcode` (the
C7-2 guardrail) fails on unreachable exported functions, and an executor with no route
to invoke it is exactly that. The choices were to add a fake caller, ship a red PR, or
land the API in the same change. The API is the honest one — an executor nobody can
invoke is not done.

**A plan is a run, so it lives in `data_migrations`.** That table already has a row per
run, a status, timestamps and a `jsonb` column. A dedicated table would have been a
second place to look for the same thing, with its own migration and its own answer to
"what is running right now?".

**But `pipeline-plan` is deliberately *not* a registry step.** Were it one, the plan's
own IN_PROGRESS row would put it in the compute lane and `LaneBusy` would block the very
steps the plan exists to run. There is a unit test asserting it is absent from both
lanes, and an integration test asserting a live plan row does not make `import` look
busy.

**One definition of what a step does.** `stepJob` is shared by the single-step handlers
and the executor. Before it, each step's work lived inline in its handler and a plan
would have needed its own copy — which is how the two would have drifted, exactly as six
parallel step tables drifted before F-1 replaced them with one registry.

**A plan confirms training on default parameters, and records that it did.** A single
step asks first and the client re-posts; a plan cannot stop to ask, and asking for the
whole pipeline *is* the confirmation. `confirm_use_default` goes into the run's args, so
"why is this model worse?" has an answer in the history rather than nowhere.

**An explicit step list is reordered into registry order**, not run as given. The
registry's order is the dependency order; honouring an arbitrary sequence would mean
running export before precompute because someone typed it that way, which the gate would
then refuse one step in — having already run the others.

**`full` excludes optional steps.** A "run everything" that silently included auto-tune
would take hours nobody asked for.

**A bug found while building R-3 on this, and fixed here.** The first version asked
tracking whether each step had *ever* completed successfully, and skipped it if so. On
any box that had run the pipeline once, a fresh `full` plan would therefore skip every
step and report success **having done nothing** — the exact silent-success failure this
codebase keeps meeting, and the worst possible one to put behind a "Run full pipeline"
button. Skipping is now driven by the prior run's own state: `Execute` runs everything,
`Resume` skips what the run being resumed completed, and the failed step is where the
resume starts rather than something to skip past.
`TestExecute_RunsEveryStepEvenOnABoxThatHasRunThemBefore` is the guard.

**A bug the tests caught.** The executor mutates its state as it walks, and handed that
same state to the `Store`. The production store marshals immediately so it never
noticed — but any implementation that *retained* what it was given would watch its
records change underneath it. The executor now clones before every store call, and
`TestCloneIsDeep` guards it.

### R-2 · API

- [x] `POST /ops/pipeline/run-plan` accepting a named plan (`full`, `retrain-only`,
      `data-refresh`) or an explicit step list — never both; the ambiguity is refused
      rather than resolved, as `/ops/data/fetch` refuses a feed and a URL together. An
      empty body means `full`, because an operator posting nothing wants the pipeline,
      not an error about which plan they forgot to name
- [x] `GET /ops/pipeline/plan` for current plan state, including `resume_from` — where
      a resume would start, answered by the same logic the executor's own skip uses.
      `POST /ops/pipeline/run-plan` takes `{"resume": true}` to continue it
- [x] `POST /ops/pipeline/stop` extended to stop the plan. The plan is cancelled
      *first*: stopping only the step it is on would end that step and then let the
      plan start the next one, which is not what Stop means

**This is the API equivalent of `make up-all` / `make full-pipeline`**, which C5-3
documented as existing only in the Makefile. It closes that asymmetry.

### R-3 · Frontend

- [x] One "Run full pipeline" action with per-step live state (`RunPlanPanel`, in the
      Ops Status pipeline card)
- [x] Resume-from-failure without restarting from the top — the button names the step
      it would resume from, and says in its description what it will skip
- [x] Show the plan even when it was started from another tab. The panel holds no plan
      state of its own: it reads `GET /ops/pipeline/plan`, which is the database. It
      polls while idle as well as while running, so a plan started elsewhere appears
      without a reload

**Progress counts finished steps, not started ones.** A bar that advanced when a step
*began* would sit at 100% while the last step was still running.

**A plan that is not running says so.** Otherwise a finished run and a stalled one look
identical, and the operator has to guess which they are looking at.

**A failed poll is not an alert.** The previous state is still the best answer available
and the next tick usually fixes it; an error banner every time the network hiccups
teaches people to ignore banners.

**Two bugs found here, both fixed where they belong.**

*In R-1, on this branch's base:* the executor skipped any step that had **ever**
completed successfully. On a box that had run the pipeline once, a fresh `full` plan
would have skipped every step and reported success having done nothing. Writing the
resume button is what made me ask what "already done" actually meant. Fixed in the R-1
commit, not papered over here.

*In this panel:* wrapping the resume button in a MUI `Tooltip` put the tooltip text on
`aria-label`, **replacing** the button's accessible name — a screen reader would have
announced "Continues from precompute, skipping what already completed" instead of
"Resume from precompute". `describeChild` makes the title a description rather than a
name. The test that caught it now asserts both.

**The failure message is rendered whole here**, not split into code/message/hint as
`OpsMigrationsTable` does. go-app already formats it as `CODE: message — hint`, so it is
actionable as it stands, and duplicating O-5's parser into a second component is how the
two would drift. Worth unifying once both have landed.

---

## Phase P — Provenance

The payoff: results become explainable.

### P-1 · Stamp provenance

- [x] Record the dataset digest in the export manifest — `export-manifest.json`, beside
      the CSVs, with the formats, the files and **their sizes**, and the dataset block
- [x] Carry it into each model's sidecar metadata alongside `feature_names`
- [x] Include the training cutoff, which already varies per run

**Provenance travels with the artifact, not with the directory that produced it.** The
export directory is overwritten by the next export; a model file that cannot say what it
trained on is one nobody can trust six months later. So the manifest is the *carrier*,
and the sidecar is where it lands.

**It sits beside `feature_names` because it answers the same kind of question:**
`feature_names` says what shape the model expects, provenance says what it learned from.

**The exporter does not read the dataset directory.** `Options.Provenance` is supplied
by the caller. An exporter that reached into the dataset directory would be an exporter
that knows about acquisition, and the layering is worth keeping — go-app's handler reads
the manifest A-2 wrote and passes the answer in.

**Read at the point of use, never cached.** The dataset directory can be replaced by an
extract between one export and the next, and a cached digest would then describe data
that is no longer there. A provenance record that is quietly wrong is worse than none —
which is the whole reason P-2 exists.

**Unknown is omitted, not blanked**, at every layer: a data directory populated by hand
has no manifest, an export from before P-1 has none, and both are recorded as absent
rather than as an object full of nulls that reads as "we looked and found nothing" when
in fact nobody looked.

**Only listed fields are copied** from manifest to sidecar. Copying wholesale would mean
a future manifest field silently appearing in every artifact written afterwards.

**Two writers, both stamped.** `training_pipeline.train_and_save` covers batting,
bowling and fielding; `artifact_sidecar.write_artifact_meta` covers extras and innings;
`train_win` builds its own metadata and is stamped directly.
`test_every_trainer_reaches_a_stamping_path` guards against a correct helper that
nothing calls — the failure no unit test would notice.

**Inference exports get no manifest.** They are inputs for a prediction, not training
data; nothing trains from them, and a manifest would invite something to try.

**Both export paths are stamped, and a test says so.** R-1's executor builds its own
export options (`stepJob`) and a manual trigger builds another (`runExportHandler`). The
first version of this item only stamped the second, so a plan-driven export would have
written a manifest naming no dataset — silently, which is the failure this phase exists
to prevent. `TestEveryExportPathStampsProvenance` fails if either call site loses it;
verified by removing one and watching it fail.

### P-2 · Surface it

- [x] Workbench shows, per model: dataset, cutoff, metrics, trained-at
      (`WorkbenchProvenanceSection`)
- [x] Flag models trained on a dataset that is no longer the live one

**Three states, not two.** A model is `current`, `stale`, or `unknown` — and unknown is
not a synonym for stale. A model trained before provenance existed, or from CSVs with no
export manifest, is *unaccounted for*, not out of date; flagging every one of them as
stale would put a warning on every model on any box that has not retrained since, which
is noise rather than a warning. The same distinction is enforced on the backend:
`dataset_is_live` is simply absent when either side is unknown.

**The comparison happens in go-app, not the browser.** ml-service can say what a model
was trained on; only go-app knows whether that is still what is on the box, because the
dataset directory is its filesystem. Computing the verdict once beats every client
deriving it slightly differently — and the frontend would have had to be told the live
digest anyway.

**Attached before the database check.** `enrichModelStatsPayload` returned early when
Postgres was unavailable; provenance now lands first, because a model being stale is
worth knowing whether or not the database is up.

**Two sidecar names, both read.** `TrainingPipeline` writes
`<kind>_metadata_<fmt>.json`; `artifact_sidecar` writes `<kind>_meta_<fmt>.json`. Two
writers, two names — unifying them would rename files that inference already reads by
name, which is a bigger change than this item, so `model_stats_service` reads either.

**Nothing to compare against is stated, not implied.** A box with no dataset manifest
gets "nothing can be compared against it" rather than a table of models silently marked
unknown — and crucially not a re-train warning, which would be the wrong advice.

---

## Cross-cutting risks

| Risk | Why it matters here | Handling |
|---|---|---|
| **Zip-slip** | A crafted archive writes outside the data directory. `archive/zip` does **not** protect you | Mandatory path sanitisation + tests with crafted fixtures (A-2) |
| **SSRF** | Server-side fetch of a user-supplied URL. Harmless on localhost, not harmless once remote | Host allowlist from day one, redirects re-checked (A-1) |
| **Disk exhaustion** | Cricsheet archives are large and expand further | Free-space precheck, size caps, staged extract with swap |
| **Long jobs vs. HTTP** | Downloads and training outlive any sane request timeout | Everything is a tracked background job returning 202 — the existing pattern |
| **Progress file races** | Two runs writing one file corrupts both | One file per run, atomic temp-and-rename writes (O-1) |
| **Partial data directory** | A failed extract leaves an inconsistent dataset that imports "successfully" | Extract to a sibling, swap on success only |
| **Silent success** | The failure mode this repo has already been bitten by twice — `make precompute` 401ing with exit 0, `.down.sql` applied forward | Every new step verifies its own postcondition: bytes match digest, entry count matches manifest, artifact exists and is non-empty |

---

## What this plan deliberately does not do

- **No raw log streaming.** Ruled out by the fidelity decision. If the milestones turn
  out to be insufficient, the smallest follow-up is switching
  `run_training_subprocess` to `Popen` with incremental reads and a per-run log sink —
  noted, not built.
- **No browser upload path.** Fetch-from-URL was the chosen source. The staging and
  extraction machinery in A-2 would serve an upload later without redesign.
- **No multi-user concurrency model.** "Localhost now" — but the global busy-lock and
  per-run progress files mean nothing here has to be unpicked to add one.
- **No scheduling / cron.** Once R-2 exists, a scheduled full run is a thin wrapper, and
  it is a different problem.
