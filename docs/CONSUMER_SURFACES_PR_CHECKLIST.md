# Workbench, Evaluate and Prediction: streamline the consumer surfaces

**Goal.** Make the three surfaces that *consume* model output — Workbench, Evaluate (DB),
Upcoming match prediction — coherent, honest about what they can actually do, and free of
paths that no longer work.

**Relationship to [OPS_CONSOLE_PR_CHECKLIST.md](OPS_CONSOLE_PR_CHECKLIST.md).** That plan
builds the *producer* side: acquisition, structured progress, run history, provenance.
This one cleans the *consumer* side. The dependency runs one way — several items here want
metrics (ops O-4) and provenance (ops P-1) to exist first. **Phases W0–W2 have no such
dependency and can start immediately.**

---

## The headline finding

**The "unified model" toggle is a guaranteed failure.** It is offered in the Evaluate tab
today and cannot succeed. Traced end to end:

1. UI offers `predictionModel: 'format' | 'unified'` (`useEvaluateDb.ts`)
2. `use_unified_model=1` → go-app `predict_team.go:297` sets `formatForPrediction = ""`
3. ml-service `resolve_model_pair` receives an empty format and raises
   **HTTP 400 `MISSING_FORMAT`** — *"Missing 'format'; models are per-format."*

This is not a regression to undo. C3-2 deliberately made an omitted format fail **loudly**,
because it previously fell back to a pooled cross-format model and quietly returned worse
numbers that looked fine. That change was correct. What was missed is that the UI kept
offering the toggle whose only remaining behaviour is to trigger the new error.

The cost of leaving it: `use_unified_model` is threaded through **20+ call sites in go-app**
and 8+ places in the frontend's `api.ts` / `types.ts`, all to reach one line that guarantees
a 400. Every reader of that code has to work out what it does before concluding it does
nothing good.

> Same class as C5-2's missing frontend step and #148's `sync-skills` no-op: the backend
> moved, the UI did not, and nothing failed loudly enough to notice.

---

## Status

| ID | Status | Depends on | Summary |
|----|--------|-----------|---------|
| W0-1 | done | — | Remove `use_unified_model` end to end |
| W0-2 | todo | — | Audit `use_latest_model` — real feature or vestigial twin? |
| W0-3 | todo | — | Sweep for other dead toggles and props |
| W1-1 | todo | — | One async-state primitive instead of 49 `useState` calls |
| W1-2 | todo | — | Shared error display that honours the structured payload |
| W1-3 | todo | — | Consolidate duplicated formatters |
| W2-1 | todo | — | Retire `WorkbenchPipelineInfoSection`'s static prose |
| W2-2 | todo | — | Audit the other prose-heavy, zero-hook components |
| W3-1 | todo | W1-1 | Split `useEvaluateDb` (365 lines, 17 states) |
| W3-2 | todo | W3-1 | Collapse the form-state / job-state duplication |
| W3-3 | todo | W1-1 | Put evaluate polling on the shared `usePolling` |
| W3-4 | todo | ops O-4 | Evaluation results: show metrics against the run that produced them |
| W4-1 | todo | W1-1 | Streamline `useUpcomingMatch` |
| W4-2 | todo | — | Prediction results: make the failure modes legible |
| W5-1 | todo | ops P-1 | Workbench as a model console: registry with provenance |
| W5-2 | todo | ops O-4 | Accuracy trend tied to actual runs |
| W5-3 | todo | ops R-3 | Retire what the ops console now shows live |

**Recommended order:** W0 → W1 → W2 in that order and immediately; W3/W4 next; W5 last,
after the ops plan's Phase O and P land.

---

## Phase W0 — Dead paths

Highest value per line changed. No dependency on anything.

### W0-1 · Remove `use_unified_model` end to end

**Why:** see the headline finding. It is a user-facing option whose only outcome is a 400.

- [x] Frontend: drop the `predictionModel` toggle from the Evaluate tab and
      `jobUseUnifiedModel` from `useEvaluateDb` — the toggle was also live in
      `UpcomingMatchTab` and `WorkbenchAccuracyTrendSection`; all three are gone
- [x] Frontend: remove `use_unified_model` from `api.ts` and `types.ts`
- [x] go-app: remove `parseUseUnifiedModel`, the `UseUnifiedModel` fields on the backtest,
      eval-job, export-contributions and predict request types, and the branch at
      `predict_team.go:295-298`
- [x] go-app: **reject** it, with `400 UNIFIED_MODEL_REMOVED` and a hint.
      Decided rather than defaulted: accepting-and-ignoring would answer with a
      different model than the caller asked for and say nothing — the silent-success
      failure this repo has already been bitten by twice. One middleware
      (`rejectRemovedParams`) covers every route, so retiring the next parameter is
      one list entry.
- [x] Update `api.test.ts` — the two tests now assert it is *not* sent
- [x] Check `model=unified` (the alias at `backtest.go:107`) is removed with it —
      and refused by the same middleware

**Note on `auto_tune`'s `unified`:** left alone, as the plan warned. It is a training
mode, not a serving tier. `removed_params.go` documents the distinction and
`TestRejectRemovedParams` asserts `?model=all&unified=1` still passes through.

**Acceptance:** no request the UI can construct results in `MISSING_FORMAT`.
**Verify:** grep for `unified` in `frontend/src` and `go-app` returns only the auto-tune
`--unified` training flag, which is unrelated — confirm that before deleting.

**Risk:** `auto_tune` has its *own* `unified` parameter (`training_orchestrator.py:302`)
that means something different — a training mode, not a serving tier. **Do not remove it.**
Two same-named concepts; this is exactly the `_LEGACY_` name collision again.

### W0-2 · Audit `use_latest_model`

**Why:** it travels everywhere `use_unified_model` does — same handlers, same job struct,
same UI hook (`useLatestModel`, defaulting true). It may be a real feature or the dead
twin. It has not been traced to a terminal effect.

- [ ] Trace it to whatever actually changes behaviour
- [ ] If it selects between artifact versions, document it and keep it
- [ ] If it has no terminal effect, remove it the way W0-1 removes its sibling
- [ ] Either way, record the finding — an untraced flag is how the first one survived

### W0-3 · Sweep for other dead toggles

- [ ] Every boolean the UI can send: trace to a terminal behaviour or delete
- [ ] Particular suspicion on options predating the per-format split
- [ ] Add a test asserting the UI's request-option set matches what handlers read,
      mirroring the ops plan's F-1 step-list test

---

## Phase W1 — Shared primitives

**Why:** three hooks hold **49 `useState` calls** between them (`useEvaluateDb` 17,
`useUpcomingMatch` 16, `useWorkbench` 16), and there is **no shared loading or error
component** — each surface rolls its own. That is why error handling is inconsistent
between tabs, and why every new panel re-invents the same three states.

### W1-1 · One async-state primitive

- [ ] A single hook owning `{ data, loading, error, run, reset }` over the existing
      `useApiCall`
- [ ] Migrate one surface first to prove the shape before converting the rest
- [ ] Target: no component holding more than one loading flag and one error string

**Risk:** a large mechanical refactor across working UI. Do it per-surface, not in one PR,
and keep each step green.

### W1-2 · Shared error display honouring the structured payload

**Why:** the backend already returns structured errors — `{code, message, hint, available}` —
and C5-2 added `CONTRIBUTIONS_CSV_MISSING` with an actionable hint. The UI mostly renders a
generic red toast, discarding the `hint` field that was written specifically to be shown.

- [ ] One error component rendering `message` prominently and `hint` as the next action
- [ ] Map known codes (`MODEL_NOT_LOADED`, `MISSING_FORMAT`, `CONTRIBUTIONS_CSV_MISSING`)
      to a concrete remedy — usually "run this pipeline step", linkable to the Ops tab
- [ ] Never swallow `available` — when a model is not loaded, listing what *is* loaded is
      the whole answer

**Acceptance:** every backend error carrying a `hint` shows that hint to the user.

### W1-3 · Consolidate duplicated formatters

- [ ] Percentage, fixed-decimal, duration and label formatters are redefined across at
      least eight components — collect into `utils/format.ts`
- [ ] One rule for null/undefined/NaN, applied everywhere

---

## Phase W2 — Embedded documentation

### W2-1 · Retire `WorkbenchPipelineInfoSection`'s static prose

**Why:** 214 lines, **zero hooks, zero API calls** — hand-written pipeline documentation
compiled into the app. It is the same failure mode as `ARCHITECTURE_MAP.md` before C7-1,
and it has already drifted: PR #149 had to correct it because it still described
`export.split_by_format` and `weather_data`, both removed.

Two forces make it worse over time: it duplicates `docs/`, and the ops console will soon
render the *live* pipeline state next to it.

- [ ] Replace the step-by-step prose with the live pipeline state once ops R-3 exists
- [ ] Keep only what is genuinely explanatory and not derivable, and link to `docs/`
      for the rest
- [ ] Anything retained that names a config key, column or endpoint must be covered by a
      test or generated — the C7-1 rule

**Acceptance:** no hand-maintained description of a config key or column ships inside a
component.

### W2-2 · Audit the other prose-heavy, zero-hook components

Found by the same scan; each needs a judgement, not a blanket rule:

| Component | Lines | Hooks | Question |
|---|---|---|---|
| `MLModelRowDetails.tsx` | 317 | 0 | Rendering real model data, or explaining it in prose? |
| `OpsMatrix.tsx` | 284 | 0 | Presentational over props is fine — confirm that is what it is |
| `WorkbenchRegistrySection.tsx` | 180 | 0 | Same question |
| `MatchScorecard.tsx` | 166 | 0 | Likely legitimately presentational |

- [ ] Classify each: presentational-over-props (**fine**) vs. embedded documentation (**fix**)
- [ ] Record the verdict so the next audit does not redo the work

---

## Phase W3 — Evaluate (DB)

### W3-1 · Split `useEvaluateDb`

**Why:** 365 lines, 17 `useState`, mixing candidate loading, evaluation jobs, polling,
scorecards and model-mode flags in one hook.

- [ ] Separate by concern: candidates, evaluation job lifecycle, scorecard
- [ ] Each built on W1-1
- [ ] `EvaluateDbSection` (434 lines) splits along the same seams

### W3-2 · Collapse the form-state / job-state duplication

**Why:** the hook holds `predictionModel` / `useLatestModel` (what the form shows) **and**
`jobUseUnifiedModel` / `jobUseLatestModel` (what the running job used), kept in sync by
`applyJobModeFromStatus`. Two sources of truth that can disagree, and the UI cannot say
which it is showing.

- [ ] One shape: form intent vs. the running job's *actual* parameters, explicitly labelled
- [ ] Show the running job's real parameters, not the current form values — after W0-1
      removes half of them there is much less to reconcile
- [ ] The reconciliation callback should disappear rather than shrink

### W3-3 · Unify polling

- [ ] `pollEvaluateStatus` is bespoke while `usePolling` exists — move it over
- [ ] Consistent backoff and stop-on-unmount
- [ ] Stop polling when the tab is hidden

### W3-4 · Results tied to the run that produced them *(depends on ops O-4)*

- [ ] Show which model artifact, dataset and cutoff produced each evaluation
- [ ] Compare against the previous evaluation of the same match
- [ ] Distinguish "model not trained" from "evaluation failed" — currently both surface
      as a generic error

---

## Phase W4 — Upcoming match prediction

### W4-1 · Streamline `useUpcomingMatch`

- [ ] 205 lines, 16 states — same treatment as W3-1
- [ ] Share candidate/team-selection logic with Evaluate where genuinely the same;
      **do not** force-share where the flows differ

### W4-2 · Make prediction failure modes legible

**Why:** prediction has real preconditions — artifacts for the format, precomputed
features, a known opposition pool. Today a missing precondition surfaces as a generic
failure and the user cannot tell which one.

- [ ] Distinguish and label: no model for format, missing precompute, unknown player,
      insufficient history
- [ ] Each maps to a named remedy (W1-2), linked to the pipeline step that fixes it
- [ ] Sequence features are zero-filled until precompute runs — say so in the UI rather
      than silently predicting on zeros

---

## Phase W5 — Workbench as a model console

Deliberately last: it is where the ops plan's provenance work becomes visible.

### W5-1 · Registry with provenance *(depends on ops P-1)*

- [ ] Per model: format, algorithm, dataset digest, cutoff, metrics, trained-at, size
- [ ] Flag models trained on a dataset that is no longer live
- [ ] Flag models older than the newest export

### W5-2 · Accuracy trend tied to runs *(depends on ops O-4)*

- [ ] Plot against real run history rather than an isolated endpoint
- [ ] Annotate points with what changed — retrain, auto-tune, new dataset

### W5-3 · Retire what the ops console now shows live *(depends on ops R-3)*

- [ ] Remove Workbench panels the Ops tab now renders from live state
- [ ] Workbench keeps one job: *what do my models look like and how good are they?*
      Anything about *running* the pipeline belongs in Ops

---

## Cross-cutting notes

**One-way dependency.** Nothing in the ops plan depends on this one. If both are in
flight, ops takes precedence on conflict.

**The recurring failure mode.** W0-1, ops F-1, C5-2's missing UI step and #148's no-op
edit are all the same shape: a backend changed, the frontend did not, and nothing failed
loudly. The durable fix is the contract tests in W0-3 and ops F-1 — cheap, and they close
the class rather than the instance.

**Test coverage is real here** — `WorkbenchTab.test.tsx`, `OpsStatusTab.test.tsx`,
`UpcomingMatchTab.test.tsx`, `api.test.ts` and others exist. Lean on them; the frontend
gate is `npm run lint` with **zero warnings tolerated** (prettier included), which has
already caught reflow slips in this repo.

**What this plan does not do:** no visual redesign, no component-library change, no new
user-facing features. Streamlining and honesty only. A redesign is a separate conversation
and should not hide inside a cleanup.
