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
| O-1 | todo | | Generalise `auto_tune_progress` into a shared progress channel |
| O-2 | todo | | Instrument the six trainers to emit milestones and metrics |
| O-3 | todo | | Serve generalised progress; fold into the existing SSE stream |
| O-4 | todo | | Persist final metrics to `data_migrations.metadata` |
| O-5 | todo | | Frontend: per-step progress, metrics, run-history drill-down |
| R-1 | todo | | Server-side run-plan executor (chaining, stop-on-failure, resume) |
| R-2 | todo | | `POST /ops/pipeline/run-plan` and plan status |
| R-3 | todo | | Frontend: "Run full pipeline" with per-step live state |
| P-1 | todo | | Stamp dataset provenance into exports and model sidecars |
| P-2 | todo | | Show which dataset produced which model |

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
- [ ] "Fetch → Extract → Import" offered as a sequence once R-1 exists — **still open,
      and deliberately.** R-1 does not exist, and chaining these in the browser would
      be a second executor that loses the run on a page reload. The tab says where
      Import is instead of pretending to sequence it

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

- [ ] Extract a shared module: `set_progress_file`, `emit(event)`, `clear`
- [ ] One progress file **per run**, not one global file — concurrent or successive runs
      must not overwrite each other's state
- [ ] Typed event schema, versioned from the start:

```json
{ "v": 1, "run_id": "...", "step": "train_batting", "phase": "cv",
  "current": 3, "total": 5, "metrics": {"rmse": 24.1}, "ts": "..." }
```

- [ ] Keep `auto_tune` working on the generalised module — do not leave two paths
- [ ] Writes must be atomic (temp file + rename) so a reader never sees a partial JSON

### O-2 · Instrument the six trainers

- [ ] `train_batting`, `train_bowling`, `train_fielding`, `train_extras`, `train_win`,
      `train_innings` emit: data loaded (rows, columns), CV fold progress, per-fold and
      final metrics, artifact written (path, bytes)
- [ ] Also emit the **dropped low-variance columns** — that is where a silently constant
      feature like `weather_composite` becomes visible instead of being quietly discarded
- [ ] Emission must never break training: wrap in try/except, log and continue

**Note:** `run_training_subprocess` keeps `capture_output=True`. Raw output stays out of
scope by the fidelity decision — the events carry the signal. Revisit only if the
milestones prove insufficient in practice.

### O-3 · Serve and stream the progress

- [ ] Generalise the auto-tune progress endpoint to `GET /admin/train/progress?run_id=`
- [ ] go-app polls it while a training step is in flight and folds the events into the
      existing `/ops/pipeline/stream` SSE payload — **one** stream for the UI, not two
- [ ] Handle the ml-service-unreachable case as *unknown*, not as failure

### O-4 · Persist final metrics

- [ ] On completion, write the final metrics into `data_migrations.metadata` (already
      `jsonb`, already the run-history table — no new table needed)
- [ ] Include the dataset digest from A-3 to close the provenance loop
- [ ] Extend `/ops/migrations` to return it

### O-5 · Frontend

- [ ] Real per-step progress bars driven by `current`/`total`
- [ ] Metrics panel per run; highlight change against the previous run of the same step
- [ ] Run-history drill-down: args, metrics, error, dataset used
- [ ] Keep the failure message actionable — the 400 `CONTRIBUTIONS_CSV_MISSING` pattern
      from C5-2 is the model to follow

---

## Phase R — Run orchestration

### R-1 · Run-plan executor

- [ ] Server-side sequential executor over an ordered step list
- [ ] Stop on first failure, leaving the plan resumable from the failed step
- [ ] Reuse `CanRunPipelineStep` for ordering rather than duplicating the rules
- [ ] Persist plan state so a page reload — or a browser closed overnight — does not
      lose the run
- [ ] Cancellation must stop the plan, not just the current step

### R-2 · API

- [ ] `POST /ops/pipeline/run-plan` accepting a named plan (`full`, `retrain-only`,
      `data-refresh`) or an explicit step list
- [ ] `GET /ops/pipeline/plan` for current plan state
- [ ] `POST /ops/pipeline/stop` extended to stop the plan

**This is the API equivalent of `make up-all` / `make full-pipeline`**, which C5-3
documented as existing only in the Makefile. It closes that asymmetry.

### R-3 · Frontend

- [ ] One "Run full pipeline" action with per-step live state
- [ ] Resume-from-failure without restarting from the top
- [ ] Show the plan even when it was started from another tab

---

## Phase P — Provenance

The payoff: results become explainable.

### P-1 · Stamp provenance

- [ ] Record the dataset digest in the export manifest
- [ ] Carry it into each model's sidecar metadata alongside `feature_names`
- [ ] Include the training cutoff, which already varies per run

### P-2 · Surface it

- [ ] Workbench shows, per model: dataset, cutoff, metrics, trained-at
- [ ] Flag models trained on a dataset that is no longer the live one

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
