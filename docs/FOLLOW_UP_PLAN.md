# Follow-up plan: two defects, metric legibility, and the accuracy roadmap

**Status: open.** Written 2026-09-02, after the P-0…P-7 re-architecture completed
([ML_PIPELINE_REARCHITECTURE_PLAN.md](ML_PIPELINE_REARCHITECTURE_PLAN.md), "the migration is
complete"). That plan's rules carry over unchanged and are assumed everywhere below: choices
are made on walk-forward folds, the locked window is scored once and never guides a choice
(H-19), sharpness at fixed calibration is the progress metric (H-22), every gate states what
varies and what is held fixed (H-23), and never commit to main.

Four threads, in the order they should land: **Plan F** fixes two defects found by using the
ops console (one of them the third instance of a known failure class, which gets a rule);
**Plan L** makes the evaluation surfaces legible to a person who has not read the plan;
**Plan M** makes the pipeline itself legible, as a map that CI will not let drift from the
code; and **Plan A** is the accuracy roadmap, taking the gains the migration identified and
deliberately left on the table.

---

## 1. Two investigations, answered first

### 1.1 D-8 — the Workbench "walk-forward registry" upload is an orphan — **fixed**

**The question it answers:** "where do I get a JSON file for the walk-forward registry?"
**Nowhere — the producer no longer exists.** The widget asks for a
`walk_forward_registry.json` that only the old `ml.walk_forward` module wrote, and P-5
deleted that module with the legacy trainers (commit `c011d23`). A repo-wide grep finds no
writer: the only mentions of the shape are the frontend parser itself
(`frontend/src/hooks/useWorkbench.ts`) and its types. The widget survived P-5's frontend
sweep because it has no backend call to fail — it parses a local file in the browser, so
nothing 404'd and no test broke.

The information it promised is not lost; it moved. Walk-forward evaluation is L4's job now:
`make evaluate` writes per-fold, per-format tables into `xi_evaluate_report.json`, go-app
proxies it at `GET /api/backtest/report`, and the Evaluation tab renders it. The fix (F-1)
is deletion plus a pointer, not a new producer.

**Fixed in F-1** (branch `fix/f-1-ops-defects`, commit `11ced17`): `WorkbenchRegistrySection`,
the upload handling in `useWorkbench` and the `WalkForwardRegistry` types are deleted, the
Workbench's Commands & docs card points at the Evaluation report tab for walk-forward numbers,
and `docs/observability.md` no longer names the widget. The frontend has no file input left.

### 1.2 D-9 — retrain from the ops console fails instantly — **fixed**

**Root cause: the two sides of one query parameter speak different date formats.**

- The console's step dialog leaves the cutoff empty by default; go-app fills it with
  `pipelinesvc.DefaultCutoff()` = `time.Now().UTC().Format(time.RFC3339)` — e.g.
  `2026-09-02T18:33:11Z` (`go-app/internal/server/step_work.go`).
- ml-service passes that string to the CLI: `python -m ml.xi.retrain --cutoff <value>`
  (`app/training_orchestrator.py`).
- The CLI parses it with `pd.Timestamp(date.fromisoformat(args.cutoff))`
  (`ml/xi/retrain.py`), and `date.fromisoformat` accepts `YYYY-MM-DD` only — verified:
  it raises `ValueError: Invalid isoformat string: '2026-09-02T18:33:11Z'` on Python 3.11.
- The subprocess exits non-zero on its first line of work, the orchestrator raises, and the
  endpoint answers `500 TRAIN_FAILED` — the instant failure observed. The endpoint's own
  hint text ("RFC3339 or YYYY-MM-DD") promises a format its parser refuses.

Typing `2025-09-01` into the dialog's cutoff box works today; leaving it empty cannot.

**Why every gate missed it — the part worth writing down.** Each component is tested against
its *own* assumption and the seam between them is tested by nothing: ml-service's tests hand
`run_retrain` a `YYYY-MM-DD` string and stub the subprocess; go-app's tests assert
`DefaultCutoff()` is valid RFC3339; the P-6 acceptance run went through the Makefile, whose
`up-all` computes `date -u +%Y-%m-%d` — the working format — so the measured 14 m 40 s chain
never exercised the console's default; and the live e2e suite that would have caught it is
gated behind `RUN_E2E=1` and does not run in CI. This is the **third** defect of the same
species, after D-7a (go-app sent numeric ids where the store was keyed by registry ids) and
the `train_`-prefix step-name mismatch (commit `996b11d`): a literal string crossing the
go-app ↔ ml-service boundary, valid on each side, meaningless in transit.

**Fixed in F-1** (branch `fix/f-1-ops-defects`, commit `11ced17`), at both ends:
`pipelinesvc.DefaultCutoff()` returns `time.DateOnly` — a retrain cutoff is a date, and the
time part named no different set of rows — and `ml.xi.retrain.parse_cutoff` truncates an
RFC3339 value to its date instead of refusing it, so an old caller or a hand-typed timestamp
cannot reproduce the failure and the endpoint's "RFC3339 or YYYY-MM-DD" hint is true. A value
that is no date at all is still refused, now with a message naming the formats that would have
worked. `ml.xi.evaluate` does not share the parse — it takes no `--cutoff`, because L4's
rolling origins are its definition (H-19) — so there was nothing to change there.

Verified the D-6 way as well as by test: against rebuilt containers, clicking **Retrain** with
the cutoff box empty starts a run — ml-service logs `cutoff: "2026-09-02"` and
`--cutoff 2026-09-02`, and the console shows the step running rather than failing instantly.

**H-24 (rule, enforced by F-1):** every literal that crosses a service boundary — step ids,
id kinds, date formats, error codes the other side matches on — is declared once in a
generated contract, and **both** sides carry a test asserting their behaviour against the
contract, not against their own copy of the assumption. The precedent exists:
`contracts/ops-console.contract.json` already does this for step ids between go-app and the
frontend; D-9 happened on an edge the contract does not yet cover. H-23 said a *gate* must
name what varies; H-24 is the same discipline for *interfaces*.

The rule is written up in [quality-and-debugging.md](quality-and-debugging.md) § H-24, with the
table of what each side asserts. F-1's audit of the boundary added three declarations to the
contract beyond step ids: `cutoff` (pattern, hint, example), `ml_service_calls` (every admin
path go-app posts to with the query parameter names it uses — asserted against ml-service's own
OpenAPI schema), and `format_codes` (go-app's `internal/formats` against `ml.xi.contract`).
Two candidates were audited and left out on purpose: go-app does not match on ml-service's error
codes (it renders `{code, message, hint}` opaquely, so there is no literal to drift), and it
does not parse run ids (it forwards them as opaque strings). Both become H-24 items the moment
either side starts matching on them.

### 1.3 D-10 — a prediction for "India" silently chose one of the two Indias — **fixed**

*This defect and D-11 were briefly numbered the other way round. F-1 recorded the
stop-that-does-not-stop as D-10 (commit `4615883`); this one was then found and written up as
D-11, on a branch already named `fix/d-10-gendered-team-resolution`. The two numbers were
swapped so each matches the branch that fixed it — `fix/d-10-gendered-team-resolution` here,
`fix/d-11-stop-that-stops` for D-11. The only stale reference left is the F-1 commit message,
which calls D-11 "D-10"; every file says what this table says.*

**Root cause: the identity migration split teams by gender and the serving path never
followed.** `opposition` has been keyed `(opposition_name, gender)` since migration `0004`,
because 130 of the 394 team names in the dataset are used by both a men's and a women's side.
The prediction request kept taking a bare name, so `FindOppositionIDForFormat` had to turn a
name into one id with no gender to go on — and it did, by **picking the side that played the
format most recently** and writing a `slog.Warn` nobody reads. Its own doc comment said so:
"the real fix is for the caller to carry the gender".

Measured against the live database on 2026-09-02, the query that resolver ran for
`("India", "T20I")`:

| club_id | gender | last_played | innings |
|---|---|---|---|
| 43 | male | 2026-07-26 | 531 |
| 132 | female | 2026-06-28 | 323 |

Every request naming India in T20I was answered with the men's side, whichever the user
meant, and the response said only `"India"` — so the answer could not be told apart from the
one that was asked for. The picker could not help: `/api/options/teams-by-format` returned
`SELECT DISTINCT opposition_name`, one entry for two teams. The ambiguity is not marginal —
names with both sides active in one format: **T20 130, T20I 31, ODI 25, TEST 4**.

Worse than the guess, `predictteam` resolved the two sides independently, so nothing stopped
a fixture from resolving to India (men) versus Australia (women). The XIs, the Elo, the
head-to-head and the context baselines would come from two different games, and the model has
never been shown such a match, so the probability it returned would be arithmetic with no
referent.

**Why the gates missed it.** It is not a seam defect like D-9; it is the *absence* of a field.
Every component behaved as specified — the resolver was tested for "unknown team is an error",
the handler for "team1 is required", the picker for "cascades formats to teams" — and no test
could fail, because no layer ever held two candidate sides at once except the one that
discarded one of them. §8.7's rule is what names it: *a substitution that changes which model
answered must say so in the response.* Substituting the men's side for the women's is a
substitution, and it was announced only in a server log.

**Fixed here** (branch `fix/d-10-gendered-team-resolution`), ends-in:

1. **The API stops guessing.** `db.ResolveTeamSide` takes a `TeamRef` — a club id, or a name
   with a gender, or a bare name — and refuses what it cannot resolve: a bare name that names
   two sides in the format returns `*db.AmbiguousTeamNameError` carrying **both** candidates,
   which the handler answers as `400 TEAM_AMBIGUOUS` with them in `available`. A bare name
   that names one side still resolves; the fix refuses doubt, not names.
2. **The response echoes what was scored.** `team1_side` / `team2_side` carry `club_id`,
   `name`, `gender` and `display_name` on **every** prediction, ambiguous or not — naming the
   side only when the request was unclear would make silence mean agreement, which is what
   this defect was. `predicted_winner` names the resolved side too ("India (women)", not
   "India").
3. **A cross-gender fixture is `400 FIXTURE_CROSS_GENDER`**, not a prediction.
4. **The options endpoint returns sides, not names**: `{club_id, name, gender, display_name}`,
   folded onto the club (`COALESCE(canonical_id, id)`) so a renamed club is listed once under
   its current name. `/api/options/opponents` takes `team_id`, since "the opponents of India"
   is the same unanswerable question one level along. The frontend picker shows "India (men)"
   and "India (women)" as distinct options and sends the id; it keeps or drops a chosen side by
   `club_id`, never by name. Venue and format flows are untouched. `/api/options/teams` (bare
   names, no format, no reader in the UI) is deleted with `db.GetUniqueTeams`,
   `GetTeamsByFormat` and `GetOpponentsByFormatAndTeam`.
5. **Every other caller.** `FindOppositionIDForFormat` had exactly two call sites, both in
   `predictteam.resolveFixture`; it is gone. The other paths were audited and have no
   ambiguity to fix, for reasons worth recording: the **importer** resolves through
   `matchIdentity.OppositionID`, which carries the match's own `info.gender`, so it has never
   guessed; **backtest** is now `GET /api/backtest/report`, a proxy to L4's report, and
   resolves no team name; the **L4 harness** reads club ids straight out of Postgres
   (`sources._MATCH_SQL` selects `COALESCE(bat.canonical_id, bat.id)`) and keys teams
   `club|gender`, so it never sees a name; and `ListPlayerPoolByOpposition` already took an id.
6. **The data check.** No team-lineage link may join two genders — one would merge two teams'
   Elo, form and head-to-head invisibly. `ApplyTeamLineage` already requires
   `predecessor.gender = successor.gender = $3`, so the importer cannot write one, but the
   column carries no such constraint. Run against the live database on 2026-09-02:

   ```sql
   SELECT p.id, p.opposition_name, p.gender, s.id, s.opposition_name, s.gender
   FROM opposition p JOIN opposition s ON s.id = p.canonical_id
   WHERE p.gender IS DISTINCT FROM s.gender;
   -- (0 rows), against 10 lineage links in total; opposition holds 370 male and 160 female rows
   ```

   **Zero bridging links, so no migration was needed.** The check is now a test against the
   migrated schema —
   `go-app/internal/db/repo_team_side_integration_test.go::TestTeamLineageNeverBridgesTwoGenders_Integration`
   — so a future mapping that bridges genders fails a run rather than silently merging.
7. **H-24.** The literal that carries gender on the wire is `team_genders` in
   `contracts/ops-console.contract.json`, generated from `go-app/internal/teams`. All three
   components assert against it: go-app in
   `pipeline.TestTeamGendersMatchTheContract`, ml-service in
   `test_both_services_match_on_the_same_team_genders` plus a test that the E7 context-group
   split keys on a value the contract publishes, and the frontend in "spells the team genders
   the way the backend does" plus a source sweep refusing any gender literal outside the one
   declaration. ml-service's `RatingState._ctx_group` now reads `C.GENDER_FEMALE` instead of
   its own `"female"` — that comparison was the third private copy of the word.
### 1.4 D-11 — "Stop pipeline" cancels go-app's job, not ml-service's training — **fixed**

Found while verifying F-1's acceptance against the containers. `POST /ops/pipeline/stop`
answered `{"cancelled": 1}` and the console stopped showing the run, but
`ml.xi.retrain` was still running inside `cric-ml-service` and still burning CPU: go-app's
cancellation closes its HTTP request to `/admin/train/retrain`, and ml-service's
`asyncio.to_thread(run_retrain, ...)` keeps waiting on a `subprocess.run` that nothing
cancels. The process had to be killed by hand.

Two things are wrong and both are one level below F-1's scope, so this is recorded rather
than fixed here: the console reports a cancellation that did not happen, and the compute
lane reads as free while a retrain is still writing — so a second retrain started
immediately would run beside the first. The fix belongs on ml-service (keep the `Popen`
handle per step, terminate it when the request is cancelled, and let the training
semaphore be released only when the process is gone), with go-app's stop waiting for that
answer instead of assuming it. Its own H-24 seam: "cancelled" is a claim one service makes
about another's process.

**Fixed in F-3** (branch `fix/d-11-stop-that-stops`), along the line that entry drew.

**On ml-service.** `run_training_subprocess` used `subprocess.run`, which keeps its handle
on its own stack — so while a retrain ran there was no object in the process that could
address it. It now uses `Popen`, registers the handle in `_TrainingProcesses` keyed by
module, and unregisters in a `finally`. `start_new_session=True` puts the child at the head
of its own process group, so a stop signals the *group*: the model-fitting workers a retrain
spawns went down with it rather than being orphaned, which was the other half of what kept
burning CPU. `stop_training` sends SIGTERM, waits `TERMINATE_GRACE_SEC` (10 s), escalates to
SIGKILL, and **waits for the process to be gone before returning** — returning on the signal
would move the same lie one layer along.

`POST /admin/train/stop` (optional `?step=`) exposes it, answering `{"stopped": [...]}` with
the steps whose process it watched exit. Nothing running is `200` with an empty list: a Stop
pressed twice is not an error. A run that ends this way raises `TrainingStopped` and is
answered `409 TRAIN_STOPPED` and logged at info — it used to come back `500 TRAIN_FAILED`
with a stack trace, which is the same species of untruth wearing a different hat.

**The compute slot, which was the subtler half.** `asyncio.to_thread` hands the event loop a
future it can cancel, but cancelling it neither stops the thread nor touches the subprocess
the thread is waiting on. When go-app dropped its request the `async with` exited, the
training semaphore was released, and the lane read as free while the retrain was still
writing — so a second retrain started then would have run beside the first.
`_run_training_step` now shields the thread's future, and on cancellation stops the process
and waits for the thread before letting the semaphore go.

**On go-app.** `StopRun` takes a `StopTrainingFunc` and returns a `StopOutcome` carrying what
it *achieved* rather than what it attempted. The remote stop goes first and on purpose:
cancelling the local job closes the request the step is waiting on, and after that the run
looks finished from here whatever is still happening over there. A stop it could not confirm
is answered `502` with `status: "partially_cancelled"` and a message saying the run was
cancelled here but the training process could not be confirmed stopped — never a plain
success. Training is compute-lane work, so a data-lane stop leaves it alone.

Note a case worse than the one recorded above, found while fixing it: with the tracking row
already gone, `cancelled` was `0`, so the old handler answered **`409 "no pipeline step is
running"`** while `ml.xi.retrain` was running. The stop now counts a confirmed training kill
as something having been stopped.

**H-24.** `/admin/train/stop` joins `ml_service_calls`, and the `stopped` field joins the
contract as `stop_response_field` — the audit under D-9 recorded that go-app parsed nothing
out of ml-service's bodies "and both become H-24 items the moment either side starts matching
on them". This is that moment. go-app asserts the *struct tag* against the contract (a
constant that agreed while the tag did not would be a green test over a stop that always read
zero steps); ml-service asserts the route exists and that its answer carries the field.

**Verified against a real retrain**, the D-6 way: `POST /ops/pipeline/run/retrain` started
`ml.xi.retrain` (pid 1984, 43 % CPU); `POST /ops/pipeline/stop` answered
`{"status": "cancelled", "training_stopped": ["retrain"]}` in 8.8 ms; the process was gone
(`returncode -15`) and `pgrep` found nothing. A second retrain started immediately afterwards,
so the lane was genuinely free. Stop with nothing running still answers `409`.


---

## 2. Plan F — fix the defects (one PR)

| id | what | why |
|---|---|---|
| F-1 | Fix D-9 at both ends under H-24; delete D-8's widget | The console's one-command pipeline is P-6's deliverable and it is broken from the UI; the orphan widget asks users for a file that cannot exist |

**F-1 steps.**
1. **D-9, both ends.** go-app's `DefaultCutoff()` returns `YYYY-MM-DD` (a retrain cutoff is
   a date; the time part was never meaningful). `ml.xi.retrain`'s `--cutoff` parser accepts
   both `YYYY-MM-DD` and RFC3339 by truncating to the date, so an old caller or a hand-typed
   timestamp cannot reproduce the failure; the endpoint hint stays true. `evaluate` takes the
   same treatment if it shares the parse.
2. **H-24 enforcement.** Add the cutoff wire format to `contracts/ops-console.contract.json`
   (e.g. `{"cutoff_format": "^\\d{4}-\\d{2}-\\d{2}$"}`); a go-app test asserts
   `DefaultCutoff()` matches the contract's pattern, an ml-service test asserts the CLI
   accepts a string matching it, and the existing frontend contract assertion picks it up
   for the dialog's date field. Audit the boundary for other uncovered literals while there
   (run ids, format codes, error codes go-app switches on) and add the ones found.
3. **Regression seam test.** Extend the e2e suite with the one case D-9 needed: POST
   `/admin/train/retrain` with go-app's *actual* default cutoff string against a stubbed
   subprocess that runs the real argument parser. Cheap, unskipped, in CI.
4. **D-8.** Delete `WorkbenchRegistrySection`, the upload handling in `useWorkbench`, and the
   `WalkForwardRegistry` types; the Workbench points to the Evaluation tab for walk-forward
   numbers. Grep for stragglers; update `docs/observability.md` if it names the widget.
5. Record D-8 and D-9 in this file's §1 as fixed with the commit; note H-24 in
   `quality-and-debugging.md` beside the test standards.

**Acceptance:** clicking Retrain with the cutoff empty starts a run (verified against the
rebuilt containers, the D-6 smoke-test way — by pointing a browser at it, not only by unit
tests); the contract test fails if either side's format drifts; the Workbench has no upload;
`make check-all` green; coverage gates never move down. **Model: Opus.**

**Met.** All five acceptance clauses hold; the browser run is recorded under D-9 above. The
ml-service coverage gate ratcheted 92 → 93 and the frontend's branch gate 77 → 78 (both to the
measured figure rounded down); go-app's stayed at 74. The seam test is
`ml-service/tests/test_ops_console_contract.py::test_retrain_endpoint_accepts_the_cutoff_go_app_sends`
— unskipped, in CI, running `ml.xi.retrain`'s real parser over the arguments the orchestrator
actually builds. One new defect was found on the way and recorded as D-11 above.

---

## 3. Plan L — legible metrics (one PR)

The evaluation surfaces are correct and illegible: `auc 0.723`, `dispersion ratio 1.02`,
`pinball` mean nothing to a reader who has not lived inside the plan. Every number the
frontend shows should explain itself on click.

| id | what |
|---|---|
| L-1 | A metric glossary as data, served with the report, rendered as click-to-open explainers on every metric, with a completeness gate |

**L-1 steps.**
1. **The glossary lives beside the code that computes the metric**, not in the frontend:
   `ml/xi/glossary.py`, one entry per reported metric key — plain-language name, a two-to-
   four-sentence explanation a non-statistician can read, the reference band (below), and
   which direction is better. The harness embeds the glossary in `xi_evaluate_report.json`
   exactly as `gates.py` embeds the H-23 triples, and the run manifest's headline metrics
   carry their keys, so one source feeds every surface.
2. **Completeness is a gate, not a habit:** a harness test walks every metric key the report
   emits and fails on any key without a glossary entry — a new metric cannot ship
   unexplained. (The pattern is H-23's registry gate, reused.)
3. **Frontend:** an info affordance on every metric label — the stat tiles, the Evaluation
   tab's tables, the Workbench manifest card, the prediction surfaces' ranges and marginal
   values — opening a popover with the entry and the reference band, the measured value
   placed on the band. One shared component; keys resolve against the glossary the service
   serves — `GET /xi/metric-glossary`, the same entries the report embeds, from the code
   rather than from a report on disk, because the prediction surfaces show metrics and
   never load that report — so the frontend holds no metric prose of its own (H-24 spirit:
   one source).
4. **Reference bands are this system's measured reality, not textbook folklore.** The
   table below is the copy the PR ships: `ml/xi/glossary.py` carries one entry per key with
   this wording, and a rewording starts here. `key` is the name the service reports the
   number under — the same key the report, the run manifest and the popover use.

| key | plain-language name | what it means | reference | better |
|---|---|---|---|---|
| `objective_auc` | Objective AUC | The value the optimiser maximises when it picks an eleven. Of two random opposing claims, how often the model ranks the actual winner higher. 0.5 is a coin flip and 1.0 is perfect. | 0.50 chance; 0.55 weak; 0.65+ useful (H-17's selection line); 0.70-0.75 is this system's measured range and the practical ceiling for cricket. Treat above 0.80 as a red flag for leakage, not brilliance. | higher is better |
| `display_auc` | Display AUC | The same measure for the probability a user is actually shown. Of two random opposing claims, how often the model ranks the actual winner higher. 0.5 is a coin flip and 1.0 is perfect. | 0.50 chance; 0.55 weak; 0.65+ useful (H-17's selection line); 0.70-0.75 is this system's measured range and the practical ceiling for cricket. Treat above 0.80 as a red flag for leakage, not brilliance. | higher is better |
| `display_auc_mean` | Display AUC (mean over seeds) | The displayed model is fitted under several random seeds and its AUC averaged, so a lucky seed cannot be read as an improvement. Of two random opposing claims, how often the model ranks the actual winner higher. 0.5 is a coin flip and 1.0 is perfect. | 0.50 chance; 0.55 weak; 0.65+ useful (H-17's selection line); 0.70-0.75 is this system's measured range and the practical ceiling for cricket. Treat above 0.80 as a red flag for leakage, not brilliance. | higher is better |
| `display_auc_seed_sd` | Display AUC spread across seeds | How far the displayed model's AUC moves when only the random seed changes. It is the noise floor: a change smaller than this is not evidence of anything (H-14). | A few thousandths here. Any claimed improvement must be larger than it. | lower is better |
| `display_auc_seed_sd_mean` | Display AUC seed spread, mean over folds | The seed-to-seed spread of the displayed AUC, averaged over the walk-forward folds. It is the size below which a difference between releases means nothing. | A few thousandths here. Any claimed improvement must be larger than it. | lower is better |
| `objective_brier` | Objective Brier | The objective's probability, scored rather than ranked. Squared error of the stated probability, averaged over matches. It rewards a probability that is both right and confident, and punishes confident and wrong. | Must beat the base-rate Brier printed beside it (about 0.25 here); this system reaches 0.20-0.22. The comparison is the point, not the absolute number. | lower is better |
| `display_brier_mean` | Display Brier (mean over seeds) | The displayed probability, scored rather than ranked, averaged over seeds. Squared error of the stated probability, averaged over matches. It rewards a probability that is both right and confident, and punishes confident and wrong. | Must beat the base-rate Brier printed beside it (about 0.25 here); this system reaches 0.20-0.22. The comparison is the point, not the absolute number. | lower is better |
| `brier` | Brier score | Squared error of the stated probability, averaged over matches. It rewards a probability that is both right and confident, and punishes confident and wrong. | Must beat the base-rate Brier printed beside it (about 0.25 here); this system reaches 0.20-0.22. The comparison is the point, not the absolute number. | lower is better |
| `base_rate_brier` | Base-rate Brier | The Brier score of predicting the training base rate for every match -- the score to beat. A model that cannot beat it has learned nothing about the fixture. | About 0.25 for a near-even base rate. The model's Brier must sit below it. | no good direction alone -- read it beside its pair |
| `specific_vs_typical_delta` | Specific-XI delta | Extra win-prediction accuracy from knowing the exact eleven, against knowing only the side's typical eleven. Above zero means the model reads the players, not the badge. | Positive means the eleven carries signal; measured +0.012 +/- 0.010 (T20) -- real but small. | higher is better |
| `delta` | Specific-XI delta | The AUC of the eleven that played minus the AUC of the side's typical eleven, in one fold. Above zero means the model reads the players, not the badge. | Positive means the eleven carries signal; measured +0.012 +/- 0.010 (T20) -- real but small. | higher is better |
| `auc_specific_xi` | AUC of the eleven that played | The objective scored on the actual elevens. Of two random opposing claims, how often the model ranks the actual winner higher. 0.5 is a coin flip and 1.0 is perfect. | 0.50 chance; 0.55 weak; 0.65+ useful (H-17's selection line); 0.70-0.75 is this system's measured range and the practical ceiling for cricket. Treat above 0.80 as a red flag for leakage, not brilliance. | higher is better |
| `auc_typical_xi` | AUC of the side's typical eleven | The same objective scored on the mean of the side's last ten aggregates instead of the eleven that played -- the control the specific-XI delta is measured against. Of two random opposing claims, how often the model ranks the actual winner higher. 0.5 is a coin flip and 1.0 is perfect. | 0.50 chance; 0.55 weak; 0.65+ useful (H-17's selection line); 0.70-0.75 is this system's measured range and the practical ceiling for cricket. Treat above 0.80 as a red flag for leakage, not brilliance. | no good direction alone -- read it beside its pair |
| `swap_violation_share` | Swap violations | How often upgrading one player -- raising his ratings, leaving the other ten and the opponent alone -- *lowers* the predicted win chance. Each one is the model contradicting itself about what a better player is. | Under 2 % passes (H-4); this system measures under 1 %. | lower is better |
| `violation_share` | Swap violations | How often upgrading one player -- raising his ratings, leaving the other ten and the opponent alone -- *lowers* the predicted win chance, in one fold. | Under 2 % passes (H-4); this system measures under 1 %. | lower is better |
| `agreement` | E5 lineup agreement | When only the eleven changed between one side's consecutive matches, how often the model's preference between the two elevens matched the direction the result moved. | Read against the derived bar printed beside it (about 0.50-0.51): an exactly-right model scores only about 0.52 on these pairs, so small margins are expected and the bar accounts for that. | higher is better |
| `bar` | E5 derived bar | The agreement a model that knows nothing would reach on these pairs, simulated from the objective's own claimed effect sizes. It is derived, never chosen, so the threshold cannot be moved to fit the answer. | About 0.50-0.51 here. The measured agreement must clear it to serve optimised selection. | no good direction alone -- read it beside its pair |
| `expected_if_exactly_right` | E5 score of an exactly-right model | What a model that is right about every pair would score, given how small the changes between consecutive elevens actually are. It is the ceiling, and it is low -- which is why the bar is low too. | About 0.52 here. A measured agreement near the bar is not a weak model; it is a small effect. | no good direction alone -- read it beside its pair |
| `simulated_mean` | E5 null mean | The mean agreement over simulated replicates of a model with no lineup knowledge. | About 0.50: the null is a coin flip on pairs whose result moved. | no good direction alone -- read it beside its pair |
| `simulated_sd` | E5 null spread | The spread of agreement across those simulated replicates -- how far the null wanders on this many pairs. The bar is a quantile of it. | Wider with fewer pairs; it is why a small format's E5 cannot decide anything. | no good direction alone -- read it beside its pair |
| `standard_error` | Standard error of the agreement | The sampling error of the agreement rate, from the number of pairs scored. A difference smaller than it is not a difference. | Read the agreement as agreement +/- this. Overlap with the bar means undecided, not passed. | lower is better |
| `ci95` | 95 % interval of the agreement | The range the agreement rate could plausibly be, given how many pairs were scored. Whether the bar sits inside it is the whole verdict. | A bar inside the interval means undecided; the pass needs the interval clear of it. | no good direction alone -- read it beside its pair |
| `effect_size` | Objective effect size on E5 pairs | How large a win-probability change the objective itself claims between the two elevens of a pair -- the median, mean and 90th percentile of the absolute change. The derived bar is simulated from these, so the model is judged against its own claim. | A few points of win probability at most; consecutive elevens differ by one to three players. | no good direction alone -- read it beside its pair |
| `within_match_spearman` | Spearman (within-match) | Rank agreement between predicted and actual player performance inside one match: did the model put the right players at the top of the card, whatever the absolute numbers were. | About 0.35 is the ceiling the game itself sets; this system reaches about 0.33. It is not expected to climb much further. | higher is better |
| `within_match_spearman_involved` | Spearman among the players involved | The same rank agreement, restricted to the players who actually did the thing -- who batted, who bowled. It is the harder and more honest version: ranking a bowler above a non-bowler is not skill. | About 0.35 is the ceiling the game itself sets; this system reaches about 0.33. It is not expected to climb much further. | higher is better |
| `top3_hit_rate` | Top-3 hit rate | How often a player the model put in a side's top three for this target really finished in the top three. A blunt, readable companion to Spearman. | No absolute target: read it against the career-mean baseline on the same rows. | higher is better |
| `mae` | Median absolute error | How far the predicted median sat from what happened, on average. It is reported for orientation only: point error is not what a distributional model is judged on (H-22). | Read against the career-mean baseline beside it, never on its own. | lower is better |
| `pinball` | Pinball loss | The proper score for a quantile forecast: it is minimised only by the true quantiles, so a model cannot buy a better score by narrowing or widening its intervals. | Only meaningful against the career-quantile baseline printed beside it; lower is better. | lower is better |
| `pinball_by_level` | Pinball loss by quantile level | The same proper score split by the quantile it scores (0.1, 0.5, 0.9), so a model that is good in the middle and wrong in a tail can be seen to be exactly that. | Only meaningful against the career-quantile baseline printed beside it; lower is better. | lower is better |
| `coverage_80` | 10-90 coverage | How often reality landed inside the stated 80 % range. It is the first question to ask of any interval: a range nobody's outcomes fall inside is not a forecast. | Nominal 0.80; within +/- 0.03 is calibrated (H-5). Far below means overconfident, far above means vague. | closer to the nominal is better |
| `coverage_80_strict` | 10-90 coverage, strict | The same coverage counting an outcome exactly on the boundary as outside. On a discrete target -- wickets, catches -- the two versions bracket the truth, and quoting only one of them would flatter or damn the model by a tie. | Read as a bracket with the inclusive coverage: calibration means 0.80 lies between them. | closer to the nominal is better |
| `width_80` | 10-90 interval width | How narrow the stated range is, in the target's own units. Narrowing is the only way to become more useful without becoming wrong. | No good absolute value: narrower while coverage holds is progress, narrower while coverage falls is a regression (H-22). Read it only beside coverage. | no good direction alone -- read it beside its pair |
| `q10` | Exceedance at the 10th percentile | The share of outcomes at or below the stated 10th percentile, counted strictly and inclusively because ties on a discrete target are real. | 0.10 when calibrated; H-5 recalibrates a target whose rate sits outside +/- 0.03 of it. | closer to the nominal is better |
| `q90` | Exceedance at the 90th percentile | The share of outcomes at or below the stated 90th percentile, counted strictly and inclusively because ties on a discrete target are real. | 0.90 when calibrated; H-5 recalibrates a target whose rate sits outside +/- 0.03 of it. | closer to the nominal is better |
| `probabilities` | Discrete-outcome probabilities | For a small-count target such as wickets, the model states a probability per outcome rather than a quantile. This block scores those probabilities and shows their calibration curve. | Judged by the Brier score and the reliability curve inside it. | no good direction alone -- read it beside its pair |
| `reliability` | Reliability (calibration curve) | Predicted probability against observed frequency, bucket by bucket. It is where a well-ranked but badly-stated probability shows itself: good AUC, wrong numbers. | Predicted about equals observed in every bucket when calibrated. | no good direction alone -- read it beside its pair |
| `vs_career_mean` | Against the career-mean baseline | The model's score minus the unconditional career mean's on the same rows, metric by metric. The baseline knows only who the player is, so this is what the fixture, the form and the conditions were worth. | Above zero on Spearman and pinball, or the model is not earning its complexity. | higher is better |
| `vs_career_quantiles` | Against the career-quantile baseline | The same comparison against a baseline that also states intervals -- each player's own career quantiles. It is the harder baseline, and the one pinball loss must beat. | Above zero on pinball, or the intervals are not worth more than the player's history. | higher is better |
| `dispersion_ratio` | Dispersion ratio | The actual spread of totals around the simulated mean, over the spread the simulator itself drew. It answers whether the simulation is as uncertain as reality is. | 1.0 calibrated; above 1 overconfident (it was 1.42 before the shared match factor); below 1 vague. | closer to the nominal is better |
| `bias` | Bias of the simulated total | Actual total minus simulated mean, averaged. It separates a level error -- every total too high -- from a spread error, which need different fixes. | Zero when unbiased; the ODI folds run -47 to +35 runs per quarter, which is a level error to chase. | closer to the nominal is better |
| `median_mae` | Median absolute error of the total | How far the simulated median total sat from the innings actually scored, on average. | Orientation only: a simulator is judged on coverage and dispersion, not point error (H-22). | lower is better |
| `actual_sd_around_simulated_mean` | Actual spread around the simulated mean | How widely real totals scattered around what the simulator expected. The numerator of the dispersion ratio. | Read only as the top half of the dispersion ratio. | no good direction alone -- read it beside its pair |
| `simulated_sd_mean` | Simulated spread | How widely the simulator's own draws scattered. The denominator of the dispersion ratio. | Read only as the bottom half of the dispersion ratio. | no good direction alone -- read it beside its pair |
| `below_q10` | PIT tail below the 10th percentile | The share of outcomes that fell below the stated 10th percentile, or above the stated 90th. It is how an interval fails: both tails fat means the intervals are too narrow. | 0.10 at each tail when calibrated; both well above 0.10 means the intervals are too narrow. | closer to the nominal is better |
| `above_q90` | PIT tail above the 90th percentile | The share of outcomes that fell below the stated 10th percentile, or above the stated 90th. It is how an interval fails: both tails fat means the intervals are too narrow. | 0.10 at each tail when calibrated; both well above 0.10 means the intervals are too narrow. | closer to the nominal is better |
| `pit_deciles` | PIT deciles | Where real totals fell in the simulated distribution, counted per decile. A calibrated simulator puts a tenth of outcomes in each; a U shape means the draws are too narrow. | 0.10 in every decile when calibrated. | closer to the nominal is better |
| `delta_brier_simulated_minus_display` | Simulated minus display Brier | How much worse (positive) or better (negative) the simulator's win probability scores than the display model's on the same matches. E2 asks exactly this. | Within 0.01 of the display model to be served as a probability; beyond it the simulation is a description only. | lower is better |
| `delta_brier_mean` | Simulated minus display Brier, mean over folds | The E2 difference averaged over the walk-forward folds -- the number the decision is actually made on. | Within the 0.01 tolerance to serve the simulated probability (E2). | lower is better |
| `delta_brier_sd` | Simulated minus display Brier, spread over folds | How much the E2 difference moves between folds. A mean inside tolerance with a large spread has not settled. | Read beside the mean: a spread larger than the tolerance means the folds disagree. | lower is better |
| `p_bat_first_wins` | Share won batting first | How often the side batting first won -- as the simulator drew it, and as it really happened. Two numbers far apart mean the simulated match, not the model's ranking, is wrong. | The simulated share should sit within a point or two of the actual share. | closer to the nominal is better |
| `max_abs_difference` | Train / serve parity (H-8) | The largest difference between a feature built by the training path and the same feature built by the as-of serving path, for the same match. It is what makes a measured number a statement about what users are served. | Exactly 0.0. Anything else is a bug, not a shade of grey. | one value is right, everything else is a bug |
| `fielded_eleven_max_abs_difference` | Previous-eleven parity | The same parity check for the elevens E5 reads from the serving path: the aggregates of a side's previous eleven must match what training saw, or the experiment is scoring a code path difference. | Exactly 0.0. Anything else is a bug, not a shade of grey. | one value is right, everything else is a bug |
| `auc` | Leak canary: a single column's AUC | How well one column alone predicts the winner over the development window. A column that should know nothing and predicts well is a leak until proven otherwise. | 0.65 or above in a limited-overs format makes the column a suspect to review (H-2). | no good direction alone -- read it beside its pair |
| `test_auc` | Leak canary: the same column in TEST | The control. A genuine signal survives in TEST; a leak that comes from the way a limited-overs row is built usually does not. | A suspect is a column at 0.65+ in a limited-overs format and 0.55 or below here (H-2). | no good direction alone -- read it beside its pair |
| `win_probability` | Win probability | The stated chance that this side wins, from the model named beside it. It is a probability, not a prediction: over many matches at 70 %, about seven in ten should be won. | Judged by Brier and reliability, not by whether the favourite won; see the evaluation report. | no good direction alone -- read it beside its pair |
| `range_10_90` | 10-90 range | The band the model expects this number to land in eight times out of ten. The point beside it is the median: a median with no range reads as a promise the model never made. | Calibrated when reality lands inside it about 80 % of the time; measured 0.786 (T20) and 0.790 (ODI) on the locked window. | no good direction alone -- read it beside its pair |
| `marginal_value` | Marginal value | The win probability the side loses if this player were replaced by an average one. It is the reason he is in the eleven, stated as a number. | A few points of win probability across a typical XI. It is a ranking aid, not a promise. | higher is better |
| `spread_share` | Spread share | How much of the innings total's uncertainty this player contributes -- his covariance with the total, over the total's variance. The shares across the eleven sum to one. | Relative: read it across the eleven, not against a threshold. | no good direction alone -- read it beside its pair |
| `spread_runs` | Spread, in runs | The same share expressed in runs: the player's share of the total's standard deviation. | Relative: read it across the eleven, not against a threshold. | no good direction alone -- read it beside its pair |
| `economy` | Economy rate | Predicted runs conceded per over: the conceded median divided by the overs the model expects him to bowl. | Format-dependent; read it against the other bowlers in the same eleven. | lower is better |

   Keys that are **not** metrics — counts, denominators, seeds, configured tolerances, the
   fitted model's own record — are declared as such in `NON_METRIC_KEYS` with a reason, so
   the completeness gate can tell a number nobody has to explain from one nobody has
   explained yet.

5. Glossary text is reviewed against `ml-and-training.md` so the two agree; the doc gains a
   one-line pointer per section to the glossary key.

**Acceptance:** every metric visible in the frontend opens an explainer; the completeness
test fails on an unglossaried key; no metric prose duplicated in the frontend;
`make check-all` green. **Model: Opus** (the bands above are the design; what remains is
carpentry). If the popover copy needs rewording, it should be reworded here first — the
table above is the source of truth the PR implements.

---

## 3b. Plan M — the system map (one PR)

The system is now small enough to describe and too layered to hold in one head: a zip becomes
event rows, becomes an as-of pass, becomes four kinds of model, becomes a selection, becomes a
verdict, becomes a run, becomes a prediction. That story is told in four documents and one
mermaid block, and none of them can say what the pipeline is *doing right now*.

| id | what |
|---|---|
| M-1 | A **System map** tab: the whole pipeline as an interactive graph, click-to-open detail on every step, structure verified in CI and numbers read live |

**The rule it is built on:** *the map may not be able to drift from the code.* A hand-drawn
diagram answers a rename by going quietly wrong — which is exactly the failure
`gen-architecture-map.py` was written to end for `ARCHITECTURE_MAP.md`. So structure is data,
verified in CI, and numbers are live.

**M-1 steps.**
1. **The graph is a contract.** `contracts/system-map.json`: nodes and edges for the full flow
   — archive → import → event tables → the as-of rating pass → the frames → the four models →
   the selector → the harness and its gates → runs and the manifest → serving → the surfaces.
   Each node carries a plain-language summary written for the same reader as the metric
   explainers, its code anchors (modules, packages, files, endpoints, tables, make targets,
   artifact kinds, pipeline steps, gates, feature groups, performance targets), the documents
   that describe it, and binding **keys** — never values. Layout is part of the contract (lane
   and column), so a moved node is a reviewable diff.
2. **The check runs both ways** (`make check-system-map`, in the Docs consistency workflow).
   Forward: everything the map names exists — a renamed module or a deleted endpoint fails.
   Reverse: everything the code can enumerate is on the map — every route both services serve,
   every step in the ops registry, every gate in the H-23 registry, every column-family
   constant in `ml.xi.contract`, every performance target, every artifact kind.
3. **Numbers are read, not written.** The tab resolves binding keys against the three endpoints
   it already has — `/ops/status`, `/api/ml/xi-status`, `/api/backtest/report` — so a figure on
   the map *is* that endpoint's figure. The harness's gates go further: the map names the gate
   ids and the report supplies each gate's terms **and the path to its own number**, so not
   even a report path is typed into the map. A key the endpoints do not carry reads as a dash.
4. **One source for metric prose.** A binding names a glossary key and L-1's `MetricInfo`
   supplies the words, resolved against the glossary the service serves — the map carries no
   metric prose of its own. The keys are checked too: a binding naming a key with no entry
   fails `make check-system-map`, because an explainer that silently does not render reads
   as "nobody has explained this yet" rather than as a typo.
5. **Read-only, and not the ops step graph.** `OpsPipelineGraph` is the control surface that
   starts and stops runs and is untouched. The system map describes the pipeline; the ops
   console drives it, and each says so on the other's node.

**Acceptance:** the tab shows the full archive-to-prediction flow; every step opens details a
non-expert can read; the numbers match `/xi/status` and the evaluation report because they are
them; deleting an endpoint or a module the map names fails CI; the ops console's step graph is
untouched; `make check-all` green; coverage gates never move down. **Model: Opus.**

---

## 4. Plan A — the accuracy roadmap

The migration's own record names where accuracy is still to be had. Ordered by expected
value over effort; each row is one PR, decided on walk-forward folds, locked window scored
once per release. Numbers quoted are the plan's (§8.3, §8.4, §8.8).

| id | what | the evidence it chases | acceptance / gate | model |
|---|---|---|---|---|
| A-1 | **Venue and competition context in L2-B** (fixture-conditional level): venue scoring rate and competition tier as as-of features of the performance model, flowing into the simulator's totals | The simulator's remaining error is *level, not spread*: per-quarter bias −47…+35 runs in ODI folds dominated by one competition; chase totals 10–16 runs high; §8.3 named this route explicitly | Per-quarter |bias| of simulated means shrinks on the folds; totals coverage holds within ± 0.03 while width does not grow (H-22); pinball no worse per target; H-21 audit for the new features | **Fable** (feature design + leakage surface) |
| A-2 | **Chase-side tails**: collapses and one-sided losses are heavier in the data than in the draws; condition the chasing innings on the target's difficulty (required rate vs as-of scoring rate) rather than only truncating at the target | Chase 10–90 coverage 0.72–0.73 from the low side — one in five real chases ends below the simulated 10th percentile; margin coverage 0.50–0.68 at nominal 0.80 | Chase coverage moves toward 0.80 ± 0.03 without first-innings coverage or E2 degrading; margins reported before/after | **Fable** |
| A-3 | **The T20 lineup signal** — the one number the migration left with a written bar it fails (E5 lineup-only 0.490 vs bar 0.501). Candidates, tested one at a time on the folds' E5, not on AUC: phase-specific matchup features (spin/pace splits per batter), role-balance interactions, and separating genuine selection from squad rotation by weighting pairs by the |Δobjective| the model itself claims | §8.8: T20 selection is scoped off until a harness run shows the objective selecting; the harness re-verdicts every run, so shipping this is flipping a measured switch, not a policy debate | T20 lineup-only E5 clears its derived bar on the folds; if no candidate does, the null is recorded per family and T20 stays rating-ordered — a recorded null is an acceptable outcome | **Fable** |
| A-4 | **Rotate the locked window.** Every model choice of the migration consulted the ≥ 2025-09-01 window; it is spent as an untouched holdout. Declare a new locked window starting at a date no decision has read (the P-7 merge date is the natural line), retire the old one into the walk-forward folds, and record the rotation policy so it happens on cadence rather than by memory | H-19's own logic: a window used for decisions is no longer clean | The harness carries the new window labelled, the old folds absorb the retired one, the policy is written in `ml-and-training.md` | **Opus** (policy + plumbing) |
| A-5 | **Data cadence**: import + retrain on a schedule (the ops chain is one command and ~12 min); more matches is the only cure for T20I's 111-pair walk-forward E5 and the women's-holdout question (H-7) | Several verdicts are power-limited, not model-limited | A documented cadence; `ratings_through` never trips H-11 in normal operation | **Opus** (ops only) |

**What not to chase**, so effort is not spent re-learning the migration's lessons: model
class (measured twice — the constraint is features, not learners); point-error metrics
(H-22); batting-order optimisation (E3 answered: median 1.5–1.7 % effect, stays with the
captain); a bigger E5 threshold (it is derived now); and any change judged on the locked
window (H-19).

**Order.** F-1 first — the pipeline must be clickable before anything retrains on cadence.
Then L-1, then M-1 stacked on it — M-1's explainers are L-1's component and its glossary
keys are checked against L-1's registry. Then A-4 before A-1/A-2/A-3, so the new modelling work is
judged against a clean window from the start; A-5 whenever convenient. A-1 and A-2 touch the
same code and should land in that order; A-3 is independent and the most likely to end in a
recorded null — which the plan treats as a result, not a failure.

---

## 5. Record of outcomes

| id | status |
|---|---|
| F-1 | **done** — `fix/f-1-ops-defects`. D-9 fixed at both ends and D-8's widget deleted; H-24 written down and enforced by a contract now covering the cutoff format, the ml-service call surface and the format codes, with an unskipped seam test. Found D-11 (a stop that does not stop), left open. |
| F-2 | **done** — `fix/d-10-gendered-team-resolution`. D-10 fixed ends-in: the prediction request names a side (club id, or name plus gender) and an ambiguous name is a 400 listing both candidates; every response echoes the sides it scored; a cross-gender fixture is a 400; the options endpoints and the picker deal in sides. `team_genders` joins the H-24 contract, asserted from all three components. The lineage data check found 0 cross-gender links in 10, so no migration. |
| F-3 | **done** — `fix/d-11-stop-that-stops`. D-11 fixed at both ends: ml-service holds the `Popen`, stops the process group and waits for it to be gone; the compute slot is held until the thread finishes, so a cancelled request no longer frees a lane a retrain is still writing in; go-app asks rather than assumes, and a stop it could not confirm is a 502 `partially_cancelled`, never a plain success. `/admin/train/stop` and `stop_response_field` join the H-24 contract. Verified against a real retrain. |
| L-1 | **done** — `feat/l-1-metric-glossary`. `ml/xi/glossary.py` carries the table in § 3 verbatim, one entry per reported metric key; the harness embeds it in `xi_evaluate_report.json` and `GET /xi/metric-glossary` serves it from the code; `glossary.check_report` walks every metric key the report emits and a harness test fails on one with no entry (keys that are not metrics are declared with a reason). One shared popover component explains every metric label in the frontend — the evaluation tables and tiles, the Workbench's manifest metrics, the run summary and the prediction surfaces' ranges and marginal values — and the metric prose the components carried was deleted. |
| M-1 | **done** — `feat/system-map-tab`, stacked on L-1 (its explainers are L-1's `MetricInfo`, and the glossary keys the map names are checked against L-1's registry). The System map tab, `contracts/system-map.json` and the two-way `make check-system-map`, wired into the Docs consistency workflow. Two things were found on the way and fixed here: `gen-architecture-map.py`'s route regex missed three go-app proxy routes (the endpoint table said 25, it is 28), and `frontend/vitest.config.ts` shadowed `vite.config.ts`, so the frontend coverage gate had never run — verified by renaming it, which turned three of the four thresholds red. The duplicate config is deleted and the gate ratcheted to the measured figures. |
| A-1 | **done — a recorded null** — `feat/a-1-venue-competition-context`, stacked on A-4. Two as-of fixture-context families are computed in the rating pass — the ground's and the competition's runs and dismissals per delivery relative to the format's, shrunk over 600 deliveries, keyed as `venue_bf_rate` keys the ground and by Cricsheet's event name (`match.event_name`) — carried on the win row and every player row on both sources, persisted with the state and compared by H-8 (max diff 0.0 on both sources). Gate A-1 (H-23 triple registered first; four arms × eleven folds × three seeds, display models and simulator seeds shared by the arms) **kept none** (plan §8.9): T20 mean per-quarter \|bias\| 3.1 → 3.2 / 3.1 / 3.1 runs (venue / competition / both), ODI 14.1 → 13.9 / 14.1 / 13.7 (−0.2 ± 0.3 and −0.4 ± 0.3 runs paired over folds, inside one standard error; the −47-run quarter reads −48 / −48 / −47), coverage, width and every pinball flat; T20I improves on every arm (7.6 → 6.7) and is reported, not decided on. The quarters with the large ODI biases are population-mix quarters — associate men's and women's ODIs scored against one baseline (2024-01: 40 matches, 11 women's, mean first innings 197) — whose ground and competition read near neutral, so §8.3's cause was wrong and the route is the baseline the elevens are measured against, not the fixture. `make evaluate` after the choice on both sources: the walk-forward table is the baseline above to every printed decimal (same configuration, same rows), the locked window still holds 0 matches, gates and glossary pass. Judgment call recorded: the registered rule had no effect-size floor and ODI's venue arm passed it on 0.2 runs; the family is not shipped on that, and the next gate of this kind states its floor first. Found B-1 (`docs/BUG_BACKLOG.md`), not fixed here. |
| A-2 | **done — a recorded null** — `feat/a-2-chase-tails`, stacked on A-1. The simulator gains a **chase response**: the chasing side's runs draws multiplied by exp(level + slope · ln r), r the target over the side's expected total on the draw's pitch, the two coefficients fitted by censored (Tobit) maximum likelihood on the shared factor's calibration fold — the shared-factor way, never set by hand — and carried in the artifact beside it (`simulator.CHASE_RESPONSE`, `FitSpec.chase_response`). Gate A-2 (H-23 triple registered first, with a level-only control arm and one fold-level standard error as the effect-size floor §8.9 asked for; four arms × eleven folds × three seeds from one fit per fold, common random numbers) **ships nothing** (plan §8.10). The slope is negative in every T20 and ODI fold (−0.8 to −2.4) and fixes what it can: chase bias −6.6 → −2.4 / +1.1 (T20), −9.3 → −0.2 / +3.7 (ODI); below-q10 0.19 → 0.14–0.15; margins 0.66 → 0.71–0.73 (runs) and 0.60 → 0.63 (balls). But chase 10–90 coverage does not move (T20 0.713 → 0.702 / 0.719, ODI 0.713 → 0.685 / 0.701; paired distance from 0.80 inside one s.e. or worse) because the mass leaves the low tail and lands above the 90th percentile (0.10 → 0.13–0.17): hard chases that were nevertheless won. E2 degrades (Brier +0.001 T20, +0.003–0.004 ODI, two to four s.e.; P(bat-first wins) moves away from the actual). The fitted residual scale (0.38–0.46 log) is nearly twice the simulated chase's spread (0.23–0.25): the miss is the chase's **dispersion** — collapse or get there — not a level by difficulty, and the level control (+0.02–0.05: the `CHASE_ORIENTATION` double-count, small and now named) moves nothing either. `make evaluate` after the choice on both sources: the walk-forward table is A-4's baseline to every printed decimal, the locked window still holds 0 matches, H-8 parity 0.0 including simulator draws, gates and glossary pass. The fit stays in the code for the next candidate (a chase-specific dispersion or mixture on the same censored sample). |
| A-3 | **done — a recorded null, per family** — `feat/a-3-t20-lineup-signal`, stacked on A-2. Gate A-3 (H-23 triple registered first: *varies* the feature family the selection objective reads, *fixed* everything else including E5's pairs and the bar's derivation, *decides* T20's fold lineup-only E5 against the bar re-derived from each arm's own claimed effect under three seeds, with a no-degradation guard on AUC and H-4 in every format) tried the three candidates one at a time on the folds (plan §8.11): **(a) phase matchup** — Cricsheet carries no bowling style, so the spin/pace split is not constructible and the per-phase batting and bowling impacts with same-phase cross terms are the matchup axis — clears T20's bar by 0.004 (0.510 ± 0.011 against 0.505–0.506, under half a standard error, +0.007 over the control) and is not shippable: it breaks H-4 in ODI (25 % of one-player upgrades lower P(win), the phase columns being collinear with the base impacts), lowers TEST's AUC by 0.007 ± 0.006 and drops T20I's agreement 0.564 → 0.486 on the one format where the objective is a proven selector. **(b) role balance** fails outright (0.496 against 0.504–0.506) and puts T20's own swap share over H-4's line (2.9 %). **(c) reweighting E5 by the claimed \|Δ\|** is a change of the measurement, not the model, and is dropped as such: T20's agreement rises with the claimed \|Δ\| (0.476 / 0.495 / 0.535 over the terciles, the rotation pairs at chance) but every tercile sits under its own exactly-right expectation, and the reweighted readings pass their re-derived bars by 0.001–0.005 on the control — inside noise, after the number was seen. The control reproduces the harness (0.503 over 2,169 pairs against 0.505–0.506; parity 0.0 on the reconstruction and on the previous elevens). T20 stays rating-ordered; the reason on the wire now quotes A-4's figure and says A-3 did not change it. `make evaluate` after the choice on both sources: see plan §8.11. Judgment calls recorded there; no bug found. |
| A-4 | **done** — `chore/a-4-locked-window-rotation`. The locked window rotates to **2026-09-02** (the P-7 merge date, the line no decision has read past); the spent ≥ 2025-09-01 window retires into four new quarterly folds (2025-09, 2025-12, 2026-03, 2026-06), so the harness runs **eleven** folds, not seven, and the retired data is scored where reuse is allowed. The report carries `locked_window` (start, rotation date, previous start, reason, retired cutoffs) and repeats both dates on every locked figure; the Evaluation tab and the System map render them. **The new window is empty** — the database ends 2026-09-01, so it holds 0 matches and says so (`n_eval: 0`, `skipped_reason`) rather than reporting noise; it fills as `make import` runs (A-5). While it cannot fit a model, H-8's parity check serves the last fold's (`parity_model_window: 2026-06-01`) and still compares 50 matches / 1,100 predictions / 50 simulations at max diff **0.0**. Policy in `ml-and-training.md` § Rotating the locked window. **Baseline for A-1/A-2/A-3** below. Run: 67 min, gates and glossary pass. |
| A-5 | open |

### A-4's baseline — what A-1/A-2/A-3 are judged against

`make evaluate` on the rotated window set (2026-09-02, run 2026-09-02, 21,093 training rows /
465,336 player rows, eleven folds 2024-01 … 2026-06, gates and glossary pass, H-8 parity 0.0).
These are **walk-forward** figures — mean ± sd over folds — because the locked window is empty
by construction until the data catches up, which is what a fresh rotation means (A-4). Read the
spreads: several are wide enough that a small move is noise.

| | T20 (11 folds) | T20I (10) | ODI (11) | TEST (11) |
|---|---|---|---|---|
| objective AUC | 0.697 ± 0.039 | 0.756 ± 0.038 | 0.673 ± 0.066 | 0.626 ± 0.113 |
| display AUC | 0.730 ± 0.050 | 0.753 ± 0.037 | 0.707 ± 0.069 | 0.646 ± 0.101 |
| swap violation share | 0.002 ± 0.003 | 0.007 ± 0.005 | 0.000 ± 0.000 | 0.005 ± 0.005 |
| specific − typical | +0.024 ± 0.018 | +0.012 ± 0.056 | +0.016 ± 0.065 | +0.004 ± 0.048 |
| **E5 lineup-only vs bar** | **0.503 vs 0.506 — fails** (n=2,168) | 0.564 vs 0.472 — passes (n=220) | 0.566 vs 0.503 — passes (n=692) | 0.549 vs 0.478 — passes (n=213) |
| runs Spearman (vs career mean) | 0.544 (+0.038) | 0.587 (+0.036) | 0.493 (+0.045) | 0.452 (+0.042) |
| runs pinball | 2.920 ± 0.117 | 3.221 ± 0.177 | 4.715 ± 0.325 | 8.328 ± 0.463 |
| runs 10–90 coverage / width | 0.897 / 29.1 | 0.889 / 31.1 | 0.901 / 47.9 | 0.774 / 82.1 |
| wickets Spearman (vs career mean) | 0.477 (−0.034) | 0.548 (−0.018) | 0.574 (+0.007) | 0.777 (+0.038) |
| E2 Δ Brier (sim − display) | +0.0016 ± 0.0063 | −0.0004 ± 0.0134 | +0.0082 ± 0.0134 | — |
| **first-innings totals** coverage / width / bias | 0.772 / 84.8 / +0.5 | 0.781 / 81.1 / +2.6 | 0.758 / 149.6 / +0.4 | — |
| **chase totals** coverage / width / bias | 0.711 / 70.1 / −6.5 | 0.689 / 66.5 / −2.7 | 0.699 / 130.2 / −9.0 | — |
| chase below q10 (nominal 0.10) | 0.192 | 0.184 | 0.181 | — |
| margin: runs when bat-first wins | 0.664 | 0.738 | 0.649 | — |
| margin: balls left when chaser wins | 0.601 | 0.648 | 0.524 | — |

**What each accuracy item has to move.** **A-1** (venue/competition context) owns the totals
rows: first-innings bias is already small in the mean (+0.5 T20, +0.4 ODI) but the per-quarter
swing is what §8.3 named, and coverage must hold within ±0.03 while width does not grow.
*(Outcome: a recorded null — the swing is a population-mix effect the fixture cannot carry;
the totals rows are unchanged. See the A-1 row above and plan §8.9.)*
**A-2** (chase tails) owns the chase rows: coverage 0.689–0.711 against nominal 0.80, and the
error is one-sided — 18–19 % of real chases finish below the simulated 10th percentile against
a nominal 10 %, the same shape §8.3 reported. Margin coverage (0.52–0.74 at nominal 0.80) is
reported beside it, not tuned. *(Outcome: a recorded null — a target-conditional response
fixes the chase's level and thins the low tail but moves the mass to the high tail, so
coverage stays; the miss is the chase's dispersion. The chase rows are unchanged. See the A-2
row above and plan §8.10.)* **A-3** owns one number: T20 E5 lineup-only **0.503 against its
derived bar 0.506** — still a fail, and now on 2,168 pairs rather than 1,358, so the verdict is
better powered than the one P-7 recorded (0.490 vs 0.501). T20I, ODI and TEST clear their bars;
TEST stays rating-ordered on H-17's AUC line, not on E5. *(Outcome: a recorded null per family —
the phase matchup clears the bar by less than half a standard error and breaks H-4 in ODI, role
balance fails, and the reweighted reading is a measurement change that passes its own bar inside
noise; the E5 row is unchanged and T20 stays rating-ordered. See the A-3 row above and plan
§8.11.)*

