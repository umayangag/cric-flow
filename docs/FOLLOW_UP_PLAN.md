# Follow-up plan: two defects, metric legibility, and the accuracy roadmap

**Status: open.** Written 2026-09-02, after the P-0…P-7 re-architecture completed
([ML_PIPELINE_REARCHITECTURE_PLAN.md](ML_PIPELINE_REARCHITECTURE_PLAN.md), "the migration is
complete"). That plan's rules carry over unchanged and are assumed everywhere below: choices
are made on walk-forward folds, the locked window is scored once and never guides a choice
(H-19), sharpness at fixed calibration is the progress metric (H-22), every gate states what
varies and what is held fixed (H-23), and never commit to main.

Three threads, in the order they should land: **Plan F** fixes two defects found by using the
ops console (one of them the third instance of a known failure class, which gets a rule);
**Plan L** makes the evaluation surfaces legible to a person who has not read the plan; and
**Plan A** is the accuracy roadmap, taking the gains the migration identified and deliberately
left on the table.

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

### 1.3 D-10 — "Stop pipeline" cancels go-app's job, not ml-service's training — **open**

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
actually builds. One new defect was found on the way and recorded as D-10 above.

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
   placed on the band. One shared component; keys resolve against the report's embedded
   glossary so the frontend holds no metric prose of its own (H-24 spirit: one source).
4. **Reference bands are this system's measured reality, not textbook folklore.** Initial
   entries and bands, drawn from the plan's record:

| metric | plain meaning | reference |
|---|---|---|
| AUC | Of two random opposing claims, how often the model ranks the actual winner higher. 0.5 = coin flip, 1.0 = perfect | 0.50 chance · 0.55 weak · 0.65+ useful (H-17's selection line) · **0.70–0.75 = this system's measured range and the practical ceiling for cricket**; treat > 0.80 as a red flag for leakage, not brilliance |
| Brier | Squared error of the stated probability; lower is better | Must beat the base-rate Brier beside it (≈ 0.25 here); this system: 0.20–0.22. The *comparison* is the point, not the absolute |
| 10–90 coverage | How often reality lands inside the stated 80 % range | Nominal 0.80; within ± 0.03 is calibrated (H-5); far below = overconfident, far above = vague |
| Interval width | How narrow the stated range is | No good absolute value: narrower **while coverage holds** is progress, narrower while coverage falls is a regression (H-22). Read only beside coverage |
| Dispersion ratio | Actual spread over predicted spread | 1.0 calibrated; > 1 overconfident (was 1.42 before the shared factor); < 1 vague |
| PIT tails | Share of outcomes below the 10th / above the 90th percentile | 0.10 / 0.10 when calibrated; both ≫ 0.10 = intervals too narrow |
| Pinball loss | The proper score for quantile forecasts; lower is better | Only meaningful against the career-quantile baseline beside it |
| Spearman (within-match) | Rank agreement between predicted and actual player performance in one match | **≈ 0.35 is the ceiling set by the game**; this system ≈ 0.33. Do not expect it to climb |
| Specific-XI delta | Extra win-prediction accuracy from knowing the exact eleven vs a typical one | Positive = the eleven carries signal; measured +0.012 ± 0.010 (T20) — real but small |
| Swap violations | How often upgrading one player *lowers* the predicted win chance | < 2 % passes (H-4); this system < 1 % |
| E5 lineup agreement | When only the eleven changed between two matches, how often the model's preference matched the result change | Read against its **derived bar** printed beside it (≈ 0.50–0.51); an exactly-right model scores only ≈ 0.52 — small margins are expected, the bar accounts for that |
| Parity (H-8) | Max difference between training-path and serving-path features for the same match | Exactly 0.0. Anything else is a bug, not a shade of grey |
| Marginal value | Win probability lost if this player were replaced by an average one | Typical XI spread: a few points of win probability; it is a ranking aid, not a promise |
| Spread share | How much of the total's uncertainty this player contributes | Relative: read across the eleven, not against a threshold |

5. Glossary text is reviewed against `ml-and-training.md` so the two agree; the doc gains a
   one-line pointer per section to the glossary key.

**Acceptance:** every metric visible in the frontend opens an explainer; the completeness
test fails on an unglossaried key; no metric prose duplicated in the frontend;
`make check-all` green. **Model: Opus** (the bands above are the design; what remains is
carpentry). If the popover copy needs rewording, it should be reworded here first — the
table above is the source of truth the PR implements.

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
Then L-1 (small, independent). Then A-4 before A-1/A-2/A-3, so the new modelling work is
judged against a clean window from the start; A-5 whenever convenient. A-1 and A-2 touch the
same code and should land in that order; A-3 is independent and the most likely to end in a
recorded null — which the plan treats as a result, not a failure.

---

## 5. Record of outcomes

| id | status |
|---|---|
| F-1 | **done** — `fix/f-1-ops-defects`. D-9 fixed at both ends and D-8's widget deleted; H-24 written down and enforced by a contract now covering the cutoff format, the ml-service call surface and the format codes, with an unskipped seam test. Found D-10 (a stop that does not stop), left open. |
| L-1 | open |
| A-1 | open |
| A-2 | open |
| A-3 | open |
| A-4 | open |
| A-5 | open |
