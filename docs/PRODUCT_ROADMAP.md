# Product roadmap: from an honest prediction system to something people pay for

**Status: Phase 0 closed on route (a), Phase 1 closed on functional acceptance, Phase 2 open
(2026-09-07).**
Written 2026-09-03. This is a business document kept to the repo's
evidentiary standard: every capability claim below is either in the measured record
([ML_PIPELINE_REARCHITECTURE_PLAN.md](ML_PIPELINE_REARCHITECTURE_PLAN.md),
[FOLLOW_UP_PLAN.md](FOLLOW_UP_PLAN.md)) or is marked as an assumption to validate. Phases
gate on evidence, the way the migration's PRs did.

**Standing constraint — a prototype on free public data (decided 2026-09-04).** The user
has scoped this system as a **prototype built from publicly available, free data**. Two
classes of work are therefore out of scope for every item in this plan and in
[EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md), whose § Explicit non-goals carries the same
rule: **paid, purchasable, subscription or account-gated data sources** — anything that
costs money, or needs an account to obtain — and **human-subject research** — interviews,
surveys, user studies. Free, publicly available, licence-clean sources only, each licence
verified at the source and recorded in [config-and-data.md](config-and-data.md) § Data-source
licence register. Everything downstream inherits it. An item the constraint removes is
marked *skipped* or *deferred* with its reason and what the skip costs, never deleted; the
constraint is the user's to lift, not this plan's.

---

## 1. The objective assessment

### What the system can honestly sell

- **Calibrated win probabilities** (display AUC 0.72–0.75, Brier beating base rate, near
  the practical ceiling for cricket) and **calibrated score/performance ranges** (totals'
  10–90 coverage ≈ 0.79, player quantiles at nominal coverage). The probabilities mean
  what they say, on both men's and women's cricket, across T20 / T20I / ODI.
- **Interactive what-if at about a third of a second per scenario** (median 355–399 ms,
  p95 471–739 ms end to end, measured in P1-2 and recorded in § 10): swap a player, change
  the venue, flip the opposition, and watch the probability, totals and scorecard move. No
  competitor consumer product offers this with honest uncertainty attached. *This line said
  ~10 ms until 2026-09-06, and 10 ms was never a scenario:* it is the simulator's draw loop
  for a fixture whose per-player forecasts have already been computed (9.4 ms measured
  in-process, matching the harness's 9.5–9.8). A user's scenario must compute those
  forecasts first, which is 296 ms of the 320 ms an ml-service `/simulate` takes.
- **A demonstrated selection edge in internationals** (E5 passed in T20I and ODI against
  a derived bar) and **explainable picks everywhere**: marginal values, spread shares,
  expected ranges, the glossary — the "why this player" card in §4 is mostly assembling
  numbers the system already computes per request.
- **A public trust asset no one else has**: the harness. "We publish our calibration,
  fold by fold, and our own report says where we are unproven" is a differentiator in a
  market of tipsters selling confidence.

### What it cannot honestly sell, and the other hard truths

- **"Better T20 XIs."** The system's own record (§8.8) says optimised selection in
  domestic T20 is indistinguishable from rating order and is scoped off. The IPL is
  domestic T20. Any IPL-facing product must be built on the *performance model and
  simulator* (valuation, projection — proven) and never marketed as XI-picking.
- **No data moat.** Cricsheet is public; the pipeline is reproducible by a competent
  team in months. The defensible assets are product execution, the trust asset, and
  (over time) proprietary operational data — squad/availability data users enter, and a
  public accuracy track record.
- **Still effectively unbenchmarked against the market.** P0-1 has now run, and the honest
  result is that it could only be run at all on 4.2 % of T20 (Big Bash and Women's Big Bash,
  the one competition pair with free licence-clean closing odds) and on no international
  match. There the market leads on both AUC and Brier, but the gap's interval spans zero. So
  the first sophisticated customer's question is answered with "we measured everything we
  could obtain freely, and it was not enough to tell" — and, with paid odds out of scope, it
  stays that way until someone publishes international closing odds under an open licence.
- **Entrenched B2B incumbents** (ball-tracking-data analytics firms) own the franchise
  market's top end. The open flank is women's cricket, associate nations and emerging
  leagues, where public ball-by-ball data is the same data everyone has.
- **Freshness is the real operating cost.** Cricsheet lags matches by days. A product
  whose ratings are stale is H-11 with a paying customer attached; near-real-time needs
  a licensed data feed (recurring cost) or honest "as of <date>" positioning. **Decided
  (P0-4, § 2.1): the honest positioning.** The recurring data cost is zero and the price
  is paid in staleness — single-digit days, measured at 2 against H-11's 14 on this box —
  and the residual exposure is the Cricsheet licence, not a subscription.
- **Regulatory adjacency.** A tool that optimises fantasy teams or resembles betting
  advice carries real compliance weight in the biggest cricket market (India). Phase 0
  was to scope this before any fantasy pivot; P0-2 is now deferred (§ 2), so it stays
  unscoped for as long as the prototype framing holds.

### Verdict

Conditional yes. Not "sell predictions" — sell the **laboratory**: an interactive,
explain-itself team workbench (prosumer), a **valuation-framed auction module** (the
differentiator, built on the proven half), and **underserved-segment B2B** (women's /
associate / emerging leagues) where the incumbents don't play and the data is level.
Accuracy-as-superiority is not the pitch; *verified honesty and interactivity* are.

---

## 2. Phase 0 — validate before building (gates, not features)

| id | what | gate it answers |
|---|---|---|
| P0-1 | **Market benchmark.** ✅ **Run, on free sources only** ([EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) § X-4). The one free, licence-clean cricket closing-odds series found is Betfair's published Big Bash / Women's Big Bash summaries; every candidate with international coverage was paid or behind a gambling account and was rejected. 588 of 592 markets joined with zero winner disagreements; 185 fall in the harness's scored windows — 4.2 % of T20, 0 % of T20I/ODI/TEST | **Answered for the hardest T20 subset only, and inconclusively.** Market AUC 0.608 vs display 0.556 (Brier 0.2387 vs 0.2488), but the gap's 95 % interval is −0.020 to +0.128. The honest sentence: *"we are not benchmarked against the market on internationals — no free source of those odds exists — and on the Big Bash matches we could price, neither we nor the market is far from a coin flip."* The limit is coverage, and under a no-paid-data rule it stays there until a freely licensed international series appears |
| P0-2 | **Legal scan** — ⏸ **deferred, not deleted** (standing constraint). Done properly it needs paid legal counsel, and the prototype pays for nothing. The scope stands as written for the day it runs: fantasy-adjacency and prediction-tool rules in target jurisdictions (India foremost), stats/name usage (facts are generally fair; images/logos are not — budget for licensing or ship without), terms for "not betting advice". It becomes a prerequisite again before anything ships publicly or takes payment | **Not answered, and known not to be.** The prototype carries **no verified disclaimer architecture and no jurisdictional clearance**. That is an accepted gap, not an answered question: which wedges are open at all is unknown, and nothing here may be presented publicly or built fantasy-facing (3c) until this runs |
| P0-3 | **Wedge interviews** — ✗ **skipped** (standing constraint: human-subject research). The 10–15 conversations — serious fantasy players, one associate-nation or women's-team analyst, one emerging-league operator — are interviews, and interviews are out of scope | **Cannot be answered.** The Phase 3 wedge cannot be chosen on evidence. Any wedge chosen later is a judgment call with no validation behind it, and has to be recorded as exactly that — the gate below says how |
| P0-4 | **Freshness decision** — ✅ **answered as a decision** (2026-09-04; the write-up is § 2.1 below). With paid feeds out of scope there was no licensed feed to quote against, so the comparison collapsed and the answer was forced: the system ships **"as of last import", with the as-of date visible**, and refuses a live prediction against ratings past H-11's limit rather than answering from a squad that has moved on. The write-up records the decision, audits the freshness machinery that actually exists (and names what does not), measures the staleness this box lives with, and puts the **Cricsheet licence question beside the cost line**, where it belongs. X-2 later put a second thing there: the weather source is free **non-commercially only**, so the zero is conditional on the use staying non-commercial (§ 2.1) | **The recurring data cost is zero, and what is paid instead is staleness.** Measured 2026-09-04 on this box: served ratings run through 2026-09-02, 2 days against the 14-day limit, and the weekly cadence bounds that at eight or nine days in normal operation. The decision is made; the *surfacing* it implies is split: the as-of date on the prediction itself, not only on the operator tabs, is **Phase 1 work (§ 3.2, P1-5)** — a Lab whose pitch is honest uncertainty cannot serve a dateless number — while the manifest and the two-rules reconciliation are Phase 2's P2-2 and P2-1 (§ 5.2); § 2.1 names all four gaps without closing them |

Exit gate: a one-page positioning statement whose every claim the harness supports, a
chosen first wedge, and a cost line. If P0-1 lands far below market and P0-3 finds no
pull, the honest outcome is "internal tool, no business" — recorded, like any null.

**Where the gate stands (closed 2026-09-06): route (a), recorded.** P0-1 ran and did not
resolve the market question; P0-2 is deferred; P0-3 — the evidence the gate's "chosen
first wedge" was to rest on — is skipped; P0-4's answer was forced and is written up
(§ 2.1); the cost line is zero by construction. The gate could not close on evidence, and
the user chose between the two honest routes through it:

- **(a) — chosen.** The prototype framing is accepted and the exit gate is recorded as
  **"internal tool / prototype, no wedge chosen"** — the outcome this section named as a
  legitimate recorded result. Said plainly: **no Phase 3 wedge is chosen**; **no evidence
  for choosing one exists**, because P0-3, the item that would have produced it, was
  skipped; and **any later choice must be recorded as an unevidenced judgment call**,
  never as a validated one. Phase 1 opens on this footing as the Team Lab alone (§ 3): an
  internal prototype has one user and needs no billing, so the SaaS half of Phase 1 is
  deferred with that reason and returns the day this route is revisited.
- **(b) — declined.** A wedge chosen by judgment and recorded as unevidenced was offered
  and not taken. It stays the other honest route if the framing changes; nothing here
  forecloses it.

§ 10 carries the closed gate.

### 2.1 P0-4 written out — the freshness decision (2026-09-04)

**The decision.** The system ships **"as of last import", with the as-of date visible and
a hard refusal behind it**. It does not pursue near-real-time. The original item was a
comparison — quote licensed feeds against honest lag — and the standing constraint above
removed one side of it: a licensed feed is a paid, account-gated source, so there is no
quote to obtain and nothing to compare. The answer is therefore forced rather than chosen,
and this write-up records it as such. What the system promises is not *current* but
*dated*: every number it serves describes the game as it stood on a date the product can
name, and a number whose date has gone past the limit is refused instead of served.

**The cost line: zero recurring, staleness paid instead.** The recurring data cost is
**zero** — Cricsheet's archive is a free download, no account, no subscription, and the
same is true of every other source the system reads
([config-and-data.md](config-and-data.md) § Data-source licence register). What is paid
instead is **staleness**. Cricsheet is a volunteer-run archive published in batches, so the
data lags the matches themselves; the product's honest positioning is a visible as-of date
and a refusal past the limit, never "live" or "near-real-time". The operating cost that
does exist is compute and an operator's attention: one cadence run a week (`make cadence`,
measured at **11 min 22 s** end to end in A-5), which is a machine that is already on.

**What freshness machinery exists today** — read from the code on 2026-09-04, not assumed:

- **The H-11 refusal.** `ml-service/app/xi_service.py`: `XiRegistry.freshness()` compares
  the loaded rating state's `last_date` against `ml.ratings_max_age_days`
  (`ml-service/config.default.json`, **14**; `XI_RATINGS_MAX_AGE_DAYS` overrides, `0`
  turns the check off). A **live** request past the limit raises `RatingsStale` →
  **503 `RATINGS_STALE`**, with a hint naming the step that fixes it. A request naming its
  own `as_of` is served, because a backtest asks for a date and gets it. The refusal and
  the reported verdict are the same computation, so they cannot disagree.
- **What the surfaces expose.** `/xi/status` carries `ratings_through` and the verdict
  object (`fresh`, `age_days`, `max_age_days`, `code`); `/health` and go-app's
  `/ops/status` (`artifacts.ratings*`) copy it through; the frontend reads it via
  `/api/ml/xi-status`.
- **What the run manifest exposes.** `ml-service/ml/xi/runs.py` — `run_id`, `created_at`,
  `cutoff`, `dataset_sha`, `git_sha`, rating params, hyperparameters, metrics,
  `state_shape` (player count and array widths). It does **not** record `ratings_through`:
  the as-of date is read from the loaded state at serve time, so a run's data date cannot
  be read from its manifest alone. On this box the served run's manifest `cutoff` is
  `2026-09-03` (the retrain's wall clock) while its ratings run through `2026-09-02`.
- **A second, independent freshness view.** go-app computes `db_freshness`
  (`go-app/internal/services/opsstatus/db_insights.go`): the latest match date per format
  straight from the database, bucketed `ok` ≤ 7 days, `stale` ≤ 30, `missing` beyond.
- **Where the date is shown in the UI.** Health tab ("Ratings: through *date* (*n* days
  old)", and the limit plus `RATINGS_STALE` when it is not fresh); Ops Status (the loaded
  run panel, and the per-format `db_freshness` grid); Workbench's "Loaded run" card
  ("Ratings through"); the System map's `ratings_through` / `ratings_age` bindings.
- **Where it is not.** On the **Upcoming match prediction** tab the date appears only
  through `PredictionReadiness`, which renders **when the prediction would be refused** —
  nothing loaded, or stale — and renders *nothing at all* when the run is loaded and
  fresh. A successful prediction is shown without a date.
- **`as_of` is not reachable from the product.** The predict request struct
  (`go-app/internal/server/predict_handlers.go`) has no `as_of` field; only backtests set
  `predictteam.Input.AsOf`. So the H-11 exemption cannot be tripped from the UI.
- **The cadence.** `deploy/cadence/` holds a systemd timer and a crontab for a weekly run
  (Monday 06:17 UTC), as documentation — **enabled by nothing**. Weekly against a 14-day
  limit leaves one missed run of slack; two consecutive misses is `RATINGS_STALE`.

**The gaps this audit found, named and deliberately not fixed here.** (1) The prediction
payload is dateless: `predictteam.Result` carries the XIs, selection, forecast, win
probability, scorecard and pools, and **no `ratings_through` and no run id** — so a number
copied, exported or screenshotted out of the product loses exactly the date the decision
says must always travel with it. **Closed by P1-5, PR #270** (2026-09-06): every
prediction carries `ratings_through` and `run_id`, required fields, stamped by ml-service
on each answer the prediction is assembled from. (2) The as-of date is absent from the one
user-facing prediction surface except when the prediction fails. **Closed by P1-5, PR
#270**: the Lab shows "ratings as of *date* · run *id*" beside the headline, off the
payload. (3) The manifest cannot answer "what
date is this run's data?" without loading the run. (4) Two freshness rules with different
inputs and different thresholds coexist and can disagree — and today they do: `db_freshness`
reads **stale** overall (TEST's latest match is 8 days old, past its 7-day bucket) while
H-11 reads **fresh** (2 days of 14). Neither is wrong; they measure different things, and
nothing reconciles them or says which one a badge would mean. **Closed by P2-1, PR
#279** (2026-09-07): `db_freshness` and its worst-of `overall` are deleted, and
`/ops/status` carries one `freshness` object every surface reads — H-11's verdict copied
through from ml-service as the only badge, the database's per-format lag as a dated fact
beside it, and `retrain_due` for the imports the served run never saw. One threshold in
the system, `ml.ratings_max_age_days`, applied where it is computed; go-app holds no copy
of it. The disagreement was reproduced on the dev stack the day it closed — TEST 11 days
behind against served ratings 5 of 14 — and now reads as one verdict with the lag stated
beside it. None belong to this item, which is a decision and an audit. Gaps (1) and (2) — the dateless payload and the dateless
successful prediction — were **Phase 1's P1-5** (§ 3.2, re-homed 2026-09-06, shipped in
PR #270): the Lab cannot honestly show a number without its date. Gaps (3) and (4) are
Phase 2's **P2-2** (the manifest) and **P2-1** (the reconciliation, the phase's priority
item), both with prompts in § 5.2.

**The staleness the prototype actually lives with** — measured on this machine on
2026-09-04, from the `cricket_data` database and the running service, not estimated. The
served run is `20260903T160602Z-0e1e39c2`, whose ratings run **through 2026-09-02**: **2
days old** against the 14-day limit, `fresh`. The database's latest match date is **also
2026-09-02** across 22,818 matches, so **nothing is lost between import and serving** — the
whole lag is Cricsheet's publication rhythm plus the time since the last cadence run. Per
format the database's latest match is T20 2026-09-02, ODI and T20I 2026-09-01, TEST
2026-08-27 (a sparse format, not a broken pipe). The archive that produced this run carried
`Last-Modified: Wed, 02 Sep 2026 16:49:50 GMT` and was fetched on 2026-09-03; its final day
is **partial** — 2026-09-02 holds 2 matches against a mean of ~9.7 per day over the
preceding 18 days — so the last fully-populated date is roughly one day earlier than the
as-of date suggests. Under the weekly cadence the age just before a run is due is therefore
about **eight or nine days** (seven of cycle plus one or two of archive tail), which sits
inside H-11's 14 with roughly a five-day margin and no room for two missed weeks. That is
the honest lag: single-digit days, always visible, never live.

**The Cricsheet licence, which belongs beside the cost line.** The zero-cost line rests on
one source whose terms are **unresolved**. Cricsheet states no licence for the match
archive; what the author states are criteria rather than a grant — free, derivatives
allowed, corrections reported, not resold — recorded in full, as read from the source, in
[config-and-data.md](config-and-data.md) § Data-source licence register (which also carries
the people register's verified ODC-By 1.0, and is not restated here). The prototype's use —
local, unsold, crediting Cricsheet, publishing no reproduction of the data — sits inside
all four criteria. This is a real operating risk, not a formality: **it must be settled
with the project directly before anything ships publicly or is sold**, which is P0-2's
row to close, and until it is, the true cost of the data is "zero, with an unpriced
licence question attached".

**And one input whose zero is conditional: the weather (X-2, 2026-09-05).** Every other
source is free on any terms it grants at all; Open-Meteo's ERA5 archive is free **for
non-commercial use only** (CC BY 4.0, verified at the source and recorded in the licence
register). For this prototype that is a zero. Were the product ever sold, the weather
would be the **first input in the system to carry a recurring cost** — Open-Meteo's
commercial plans are the paid route — so the line "recurring data cost: zero" holds only
while the use stays non-commercial. Today nothing depends on it: X-2 measured the four
weather families and shipped none ([EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) § X-2),
so the acquired days and coordinates are kept under `reference-data/` as product data (a
venue's whereabouts, a match day's weather) rather than as a model input, and dropping
the source would cost no accuracy.

## 3. Phase 1 — the Team Lab (the product core)

**Opened 2026-09-06, scoped to the Team Lab only.** Phase 0 closed on route (a) (§ 2): an
internal prototype with one user. Phase 1 is therefore the client-facing workbench and
nothing else, on the existing serving stack — go-app + ml-service already answer every
call this needs, and the work is product engineering. The item-by-item kickoff prompts
are in § 3.2; the ground truth they build on is in § 3.1.

**What Phase 1 builds** — the five items of § 3.2:

- **Pool picker and Optimise** (P1-1). Pool picker (search players, or start from a real
  squad), opposition pool, venue, date, format; **Optimise** returns the XI with win
  probability, simulated totals and scorecard. The **toss toggle** (bat first / bowl
  first / unknown) lives here, wired to `team1_bats_first`; "unknown" is today's
  behaviour — the simulator marginalises over the toss — so the toggle exposes a choice
  the stack already makes rather than adding one. *Read from the code on 2026-09-06:* the
  parameter reaches `POST /simulate` and `predictteam`'s simulation input
  (`xi_simulation.go`), but the public predict request (`predictTeamRequest`) has no
  field for it and the UI has no control. D-12 wrote the toggle into this spec and shipped
  the pool machinery beneath it — the recency-bounded pool, optional manual picking via
  `GET /api/options/candidates`, the retirement ledger — not the toggle itself. The
  HTTP-boundary field and the control are P1-1's work.
- **Play mode** (P1-2, **shipped 2026-09-06**): add/remove/swap players on either side;
  every change re-scores live and shows the delta; constraint chips (keeper, bowlers,
  must-include). The displayed probability is monotone under that swap **by
  construction**: upgrading a player never lowers it, in any format, measured at 0.0000
  violations per fold. It used to move the wrong way 3–7 % of the time, and buying the
  guarantee cost display AUC in ODI and TEST (`docs/BUG_BACKLOG.md` § B-7) — a deliberate
  trade, made because this bullet is the product. P1-2 demonstrated it on the surface for
  a swap that is an upgrade on *every* rated axis, and found the limit of the claim: a
  player the rating order merely ranks higher is not one (B-8).
- **The "why this player" card** (P1-3, specified in § 4, **shipped 2026-09-06**) on every
  selected player, under the rule that it shows only what the objective consumed. Four
  fields § 4 first sketched are omitted with their reasons recorded there, because the
  stack cannot produce them from a consumed input.
- **Honesty built into the UI, not the footnotes** (P1-4): ranges always shown, the
  not-optimised notice where selection is rating-ordered (T20, TEST), "ratings as of
  <date>", the metric explainers (L-1) reused throughout.
- **The as-of date on a served prediction** (P1-5). P0-4's audit (§ 2.1) found the
  prediction payload dateless and the Upcoming-match tab dating only its refusals; for a
  Lab whose pitch is honest uncertainty that is a Phase 1 gap, not a Phase 2 badge.

**Deferred with the reason recorded — the SaaS half of Phase 1.** Kept visible so nothing
is silently lost; none of it is being built:

| deferred | why it waits |
|---|---|
| Accounts / auth | An internal prototype has one user; there is nobody to authenticate |
| Per-user saved scenarios | One user and no accounts to key them by; the Lab's state lives in the browser session |
| Rate limits, usage metering | Nothing to meter and nobody to bill; the number they would produce — infra cost per active user — has no denominator |
| Production hosting, uptime monitoring, the run-manifest discipline in production | Hosting is a running cost the prototype pays nothing for; the dev stack (`make dev-up`) is the deployment |
| Freemium / subscription tiers (N scenarios/day free; unlimited + saved squads + exports paid) | No billing without accounts, hosting and P0-2's legal clearance, each of which is itself deferred |

Every row returns the day route (a) is revisited (§ 2). The standing constraint at the
head of this document is the user's to lift, not this plan's.

**Exit gate — functional acceptance (re-specified 2026-09-06).** The gate as first
written — strangers use it unassisted, retention measured in week 3, infra cost per active
user — is a user study, which the standing constraint rules out, and is meaningless for an
internal tool with one user. It is replaced, not softened: every clause below is a thing a
worker can demonstrate on the dev stack. Phase 1 is done when all six hold, and § 10
records it:

1. **Correctness against the serving stack.** ✅ **Held across P1-1, P1-2 and P1-4**
   (2026-09-06): the XI, the win probability, the totals and the scorecard the Lab shows
   are the ones `POST /api/predict/team-selection` returns for the same inputs, with no
   client-side arithmetic on top of them. P1-1 verified the served numbers on eight
   fixtures through the endpoint; P1-2 showed there is one code path for them — pinning
   the searched eleven reproduces Optimise's own probability with a gap of
   **0.0000000000** in all four formats; P1-4's inventory walked every served state and
   found nothing on the surface adjusting what the stack served.
2. **Latency, measured rather than asserted.** ✅ **Measured in P1-2, and the claim was
   corrected** (2026-09-06): a Play-mode re-score is **355–399 ms median, 471–739 ms p95**
   end to end as the user sees it — the round trip through go-app to ml-service and back —
   against the ~10 ms this document used to claim, which was the simulator's in-process
   draw loop for a fixture whose forecasts were already computed. § 1 and § 10 now carry
   the measured figures and the attribution. The clause stands as *measured and recorded*,
   not as a threshold: no target was set for it before the measurement, and setting one
   afterwards would be fitting the bar to the number.
3. **A swap never moves the win probability the wrong way.** ✅ **Demonstrated on the
   surface in P1-2** (2026-09-06): swapping in a player who is at least as good on *every*
   as-of vector the store holds never lowered the displayed probability — 0 falls in 20
   such swaps across the four formats, through the real stack — and pinning the searched
   eleven reproduces Optimise's own probability to the last digit, so the two are one code
   path. What the same probe also found, and what this clause never claimed: a swap for a
   player the *rating order* merely ranks higher can lower it (up to −0.08 in TEST), because
   that order is one composite and the display model reads several axes. That is recorded
   as B-8, not as a broken guarantee.
4. **The honesty surfaces are present.** ✅ **Swept in P1-4** (2026-09-06): ranges always
   shown; the not-optimised notice wherever selection is rating-ordered (T20, TEST), from
   `selection.note`; "ratings as of <date>" on every served prediction through one
   component; the L-1 explainers reachable from every labelled number, the two sources
   (win probability, forecast) included. **Three states the inventory found that this
   clause did not anticipate, now part of it:** (a) *Play mode's selection is a third
   state*, `objective: fixed` — neither searched nor rating-ordered — and it carries its own
   notice ("your eleven"), its own heading and its own board-order sentence, because
   labelling a hand-built eleven "rating-ordered" (which the Lab did) is a false claim
   about the policy; (b) *a point the stack serves with no range* — wickets on the
   performance-quantiles path (TEST) come as an expectation with P(0/1/2+) and no
   quantiles — reads "3.6 (no range)" and never a bare point, and the surface derives no
   interval from the pmf; (c) *a must-include id on Optimise* — P1-4 found it joined
   the pool and was not enforced, made the label say so and put the outcome on the wire
   (`selection.must_include`); **B-10 then made it true** (2026-09-06), so the ids are a
   lock the search honours, the input says "required in the eleven", a request no eleven
   can satisfy is refused with its reason, and the wire check stays as the postcondition. A served answer with no
   `selection.note` renders the notice saying no reason was served rather than a sentence
   the surface invented.
5. **The card shows only what the objective consumed.** ✅ **Built in P1-3** (2026-09-06):
   the why-this-player card carries no input the objective did not read, every field maps
   to a named computation, and four fields § 4 first sketched are omitted with their
   reasons recorded there rather than approximated. The rating-ordered card says "picked by
   rating, not by the win model" and carries nothing the win model would have said.
6. **Failure paths are honest.** ✅ **Demonstrated in P1-4** (2026-09-06): stale ratings
   are refused with `RATINGS_STALE` and the refusal is shown with its date; nothing loaded
   is shown as nothing loaded (`XI_MODEL_UNAVAILABLE`, and the readiness notice before a
   request says there is no date to show); a cross-gender fixture is `400
   FIXTURE_CROSS_GENDER`; **an unreachable ml-service is `502 ML_UNREACHABLE`** with the
   endpoint and the transport's reason — it used to reach the Lab as `500 INTERNAL` with a
   dial string for a message, which blamed the wrong service and named no remedy. No state
   is quietly served as a prediction, and no failure state shows a number or a date it
   does not have.

What this gate does not measure, said so nobody reads it as more: retention, pull, cost
per user — every number a product gate would carry and a prototype cannot.

**Where the gate stands (closed 2026-09-06): functional acceptance, recorded.** All five
items shipped (P1-1 #269, P1-5 #270, P1-2 #271, P1-3 #272, P1-4 #273) and each of the six
clauses above is marked with the item that demonstrated it on the dev stack. The user has
closed the phase on that evidence.

What was accepted, stated at its actual size: **an internal prototype whose surfaces do
what this section says they do**, demonstrated against P1-4's inventory of every served
and refused state rather than asserted. That is the whole claim. It is **not** a claim
that anyone wants the Lab, would keep using it, or would pay for it: retention, pull and
cost per active user were never measured, and route (a) put all three out of scope (§ 2)
along with the user study the original gate rested on. Phase 1 closing therefore says the
build is done and honest, and says nothing at all about the market — the questions P0-1
left open and P0-3 could not ask are still open, and closing this gate does not touch
them.

Two things were left open by the phase and are recorded rather than folded into the
acceptance: **B-8** — the selection's rating order can disagree with the display model, so
a swap for a higher-*rated* player can lower the probability; surfaced honestly on the
surface in P1-3 and P1-4 and not fixed — and **B-10**, must-include reaching the search as
an empty lock, which is fixed on the branch that records this closure. Both live in
`docs/BUG_BACKLOG.md`.

### 3.1 What Phase 1 inherits

Ground truth the five items build on, recorded here so no worker rediscovers it:

- **Selection is rating-ordered in T20 and TEST, optimised in T20I and ODI.** TEST has no
  win objective that ranks (H-17); T20's objective ranks but has not shown it selects — E5
  lineup-only agreement 0.503 against the derived bar 0.506 on the rotated folds, and
  neither of A-3's feature families moved it (§ 8.8 and § 8.11 of
  [ML_PIPELINE_REARCHITECTURE_PLAN.md](ML_PIPELINE_REARCHITECTURE_PLAN.md)). The policy
  is `ml/xi/optimizer.py` `NOT_OPTIMISED_REASONS`, served on the wire as
  `selection.optimised = false` with the reason in `selection.note`, and a rating-ordered
  XI carries no `marginal_value` because nothing was maximised. The not-optimised notice
  exists because of this: the Lab must never search the win model where the policy says
  not to, and must never hide that it did not.
- **The display model's swap-violation share is 0.0000 in every format** after PR #267
  (`docs/BUG_BACKLOG.md` § B-7): the display model no longer reads the Elo spread, and a
  one-player upgrade never lowers the displayed probability. Play mode's core interaction
  is coherent by construction, at a recorded cost — display AUC −0.0022 ODI / −0.0059
  TEST on the confirming database run, against the gate's priced −0.0055 / −0.0047. A
  worker who sees a swap move the wrong way on the surface has found a plumbing bug, not
  a model property.
- **The serving stack already answers every call the Lab needs.** go-app:
  `POST /api/predict/team-selection` (both XIs with per-player points and 10–90 ranges,
  `selection`, `forecast`, `win_probability` with its source, `scorecard`, both pool
  summaries with the window, size and named exclusions), `GET /api/options/{formats,
  teams-by-format, opponents, venues, candidates}`, `POST /api/players/{id}/retirement`,
  `GET /api/ml/xi-status` (the loaded run, `ratings_through`, the freshness verdict),
  `GET /api/backtest/metric-glossary` (L-1). ml-service beneath it: `/xi/optimize`,
  `/xi/predict-win`, `/simulate`, `/performance/predict`, `/xi/status`,
  `/xi/metric-glossary`. The frontend already has the Upcoming-match tab,
  `CandidatePoolDialog`, `useCandidatePool`, `useVenueSearch`, `MatchScorecard` (with
  its 10–90 ranges) and `MetricGlossaryContext`. No item in § 3.2 adds a model; the model
  layer is done.
- **The simulator's day/night calibration gap is known, recorded and out of scope.** X-2's
  H-22 split ([EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) § X-2) found the control
  simulator too narrow by day and too wide at night — T20 first-innings 10–90 coverage
  0.734 vs 0.841, ODI 0.715 vs 0.932, against a nominal 0.80 — and no weather family
  touched it. It belongs to a future simulator item, not to Phase 1: the Lab shows the
  ranges the simulator produces and must not paper over them — no client-side widening or
  narrowing, no day/night adjustment, no hiding of a range.

### 3.2 Phase 1 kickoff prompts

One prompt per item, in the [EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) style: run each
in a fresh chat with the model noted, and each ends by handing over the push and PR
commands. Run order: **P1-1 → P1-5 → P1-2 → P1-3 → P1-4** — the shell first, the date
next because every later surface shows it, then the interaction, the card, and last the
sweep that checks every honesty surface is in place. These are product items, so each
gate is functional acceptance, not a measured null. Every prompt carries the same rules
in its own words — branch off `main` as a named feature branch and never commit to
`main`; conventional commits with scope; anchored edits; `make check-all` green and
coverage gates never moving down; the database rule; the checkpoint drill — so that a
fresh worker needs nothing but the prompt.

#### P1-1 — Pool picker and Optimise: the Lab's shell, and the toss toggle (model: Opus) — **shipped**

*Shipped on `feat/p1-1-team-lab-shell` (2026-09-06). What shipped and what was seen on the
dev stack is in § 10's P1-1 row.*

**What.** The Team Lab surface itself, grown out of the Upcoming-match tab rather than
built beside it: format, team and opposition pools (search, or a real squad through the
D-12 candidate list), venue, date, constraints, and **Optimise** returning both XIs, the
win probability, the simulated totals with ranges and the scorecard — every number from
the response, none computed in the client. The toss toggle is here because it is the one
input the spec names that the HTTP boundary and the UI do not yet carry. **Gate:** one Lab
surface on the dev stack answering every input above from the existing calls; the toss
toggle's three states reach the simulator and are named on the surface; every pool
visible with its window, size and exclusions; nothing on the surface computed client-side.

```
Read docs/PRODUCT_ROADMAP.md § 3 (Phase 1 — the Team Lab: what it builds and its gate),
§ 3.1 (what Phase 1 inherits) and § 3.2's P1-1 entry; docs/EXTERNAL_DATA_PLAN.md § D-12
and its record (the pool machinery this builds on). Read the Upcoming-match surface as it
stands: frontend/src/components/UpcomingMatchTab.tsx, CandidatePoolDialog.tsx,
MatchScorecard.tsx, PredictionReadiness.tsx, hooks/useUpcomingMatch.ts,
useCandidatePool.ts, useVenueSearch.ts; go-app/internal/server/predict_handlers.go
(predictTeamRequest), candidate_handlers.go, ml_xi_client.go;
go-app/internal/services/predictteam/ (predict_team.go: Result, SelectedPlayer,
SelectionSummary; pool.go: PoolSummary; xi_simulation.go: Team1BatsFirst);
ml-service/app/models/xi.py (SimulateRequest.team1_bats_first). Branch off main as
feat/p1-1-team-lab-shell.
Rules: never commit to main; branch off main as the feature branch named above;
conventional commits with scope; anchored edits; make check-all green and coverage
gates never move down — ratchet them up when coverage rises (go-app/Makefile COV_MIN +
root Makefile + .github/workflows/go-app-ci.yml move together; likewise ml-service;
frontend/vite.config.ts thresholds); H-24 for any new wire literal; §8.7 — every
substitution or fallback is visible on the wire, never only in a log. Database rule:
cricket_data holds 22,818 matches, 11,539,808 ball events and 13,662 player_biography
rows — read it freely, run nothing destructive against it; anything destructive goes to
the scratch database cricket_flow_test (make -C go-app test-db;
dbtest.SkipUnlessScratchDatabase), and you verify those three counts unchanged before
you hand over. Checkpoint drill: if usage nears the cap, commit, write RESUME_NOTES.md
with every number already measured, push, and keep going; never report a partial item
as complete.

Do P1-1: the Team Lab's shell — one surface, on the existing calls, adding no model.
1. THE SURFACE. The Team Lab is the Upcoming-match tab grown up, not a parallel tab:
   rename and extend it (the project is not live; no backward compatibility) so there is
   one surface answering POST /api/predict/team-selection, never two that can drift. Keep
   the hooks; logic lives in hooks and pure functions, components stay presentational.
   Inputs: format; team — search, or start from a real squad through the D-12 candidate
   list (GET /api/options/candidates: recency pool by default, all-time one click away,
   manual ticking optional); the opposition pool the same way; venue; date; constraints
   (min bowlers, keeper, must-include via the extra ids). Optimise runs the call and shows
   the XI for both sides, the win probability with its source named, the simulated totals
   with their 10-90 ranges, and the scorecard — the numbers the response carries, no
   client-side arithmetic on top of them.
2. THE TOSS TOGGLE. Bat first / bowl first / unknown, default unknown. Read the code
   first: team1_bats_first reaches ml-service's POST /simulate and predictteam's
   simulation input (xi_simulation.go), but predictTeamRequest has no field for it and the
   UI has no control. Add the field at the HTTP boundary (nullable; absent means unknown,
   which is today's marginalised behaviour), thread it to the simulation input, add the
   control, and show on the surface which toss the numbers assume — the response already
   carries scorecard.toss_marginalised, and a known toss must read as known. A handler
   test pins each of the three states.
3. THE POOL, VISIBLY. Render both PoolSummary objects as D-12 specified: "pool: played for
   <team> in the last <N> months (<M> players, <K> excluded as retired)", the excluded
   players struck through with the reason and reversible from the surface, all-time and
   manual one click away. Nothing about a pool is silent.
4. VERIFY the D-6 way: make dev-up, predict a real upcoming fixture in each of T20I and
   ODI (optimised) and T20 and TEST (rating-ordered), and look: the XI, the probability,
   the ranges and the scorecard are on the surface; a rating-ordered format shows
   selection.optimised false and its note (P1-4 polishes the notice; here it must be
   visible and never dropped); the three toss states produce three responses and the
   surface says which it is showing. Record what you saw in the PR.
5. TESTS AND DOCS. Handler tests for the toss field (httptest), hook and component tests
   for the inputs and the pool rendering (Testing Library, no implementation details);
   the frontend-backend sync check stays green. Update README.md's tab list and, if the
   contract moved, make gen-architecture-map.

Acceptance (the P1-1 gate): one Lab surface exists on the dev stack and answers every
input above from the existing calls; the toss toggle's three states reach the simulator
and are named on the surface; every pool is visible with its window, size and exclusions;
no number on the surface is computed client-side; make check-all green; coverage gates
never move down; cricket_data's three counts unchanged. Record what shipped in
docs/PRODUCT_ROADMAP.md § 10 (the P1-1 row). Then stop and hand over the push and PR
commands.
```

#### P1-5 — The as-of date on a served prediction (model: Fable) — **shipped**

*Shipped on `feat/p1-5-prediction-as-of-date`, PR #270 (2026-09-06). What shipped and what
was observed on the dev stack is in § 10's P1-5 row.*

**What.** Close gaps (1) and (2) of P0-4's audit (§ 2.1): `predictteam.Result` carries no
`ratings_through` and no run id, and the Upcoming-match tab shows the date only when the
prediction would be *refused*. Every successful prediction gains its date and run id, read
from the same source `/xi/status` reads so the two can never disagree, and the surface
shows them beside the headline; the `RATINGS_STALE` refusal is demonstrated end to end.
Gaps (3) and (4) stay Phase 2's. **Gate:** every successful prediction on the surface
carries the date and run id it was served from, read from its own payload; a stale
registry is refused with `RATINGS_STALE` and the refusal is shown with the date, never a
quietly served number.

```
Read docs/PRODUCT_ROADMAP.md § 2.1 (P0-4's audit: gaps (1) and (2) are this item's; (3)
and (4) are Phase 2's), § 3 (the Phase 1 gate's clauses 4 and 6) and § 3.2's P1-5 entry.
Read go-app/internal/services/predictteam/predict_team.go (Result),
go-app/internal/server/predict_handlers.go and ml_xi_client.go,
ml-service/app/xi_service.py (XiRegistry.freshness; RatingsStale -> 503 RATINGS_STALE),
ml-service/app/models/xi.py (XiStatusResponse: ratings_through and the freshness
verdict), ml-service/config.default.json (ml.ratings_max_age_days, 14;
XI_RATINGS_MAX_AGE_DAYS overrides) and frontend/src/components/PredictionReadiness.tsx
(the date shown only when the prediction would be refused). Branch off main as
feat/p1-5-prediction-as-of-date.
Rules: never commit to main; branch off main as the feature branch named above;
conventional commits with scope; anchored edits; make check-all green and coverage
gates never move down — ratchet them up when coverage rises (go-app/Makefile COV_MIN +
root Makefile + .github/workflows/go-app-ci.yml move together; likewise ml-service;
frontend/vite.config.ts thresholds); H-24 for any new wire literal; §8.7 — every
substitution or fallback is visible on the wire, never only in a log. Database rule:
cricket_data holds 22,818 matches, 11,539,808 ball events and 13,662 player_biography
rows — read it freely, run nothing destructive against it; anything destructive goes to
the scratch database cricket_flow_test (make -C go-app test-db;
dbtest.SkipUnlessScratchDatabase), and you verify those three counts unchanged before
you hand over. Checkpoint drill: if usage nears the cap, commit, write RESUME_NOTES.md
with every number already measured, push, and keep going; never report a partial item
as complete.

Do P1-5: a served prediction carries its date, and a refused one says why.
1. THE PAYLOAD. predictteam.Result gains ratings_through (YYYY-MM-DD, the date the served
   ratings run through) and run_id (the loaded run), read at serve time from the same
   place /xi/status reads them — one source, so the payload and the status can never
   disagree. Decide on the go-app side where they come from (the ml-service responses the
   prediction already makes, or one status read per prediction) and say why in the
   commit; do not cache a date past the request. Both fields are required, not omitempty:
   a prediction without a date is the defect this item closes.
2. THE SURFACE. The Lab (P1-1's surface) shows "ratings as of <date> - run <id>" on every
   successful prediction, beside the headline probability, from the payload — not from a
   separate status poll that could describe a different run. PredictionReadiness keeps
   its job for the refused states; the successful state now has a date too, so the
   surface is dateless in no state.
3. THE REFUSAL. Confirm end to end that a live prediction past ml.ratings_max_age_days is
   refused with 503 RATINGS_STALE through go-app to the surface, and that the surface
   shows the refusal with the date, the age against the limit, and the step that fixes
   it — never a stale number. Demonstrate it on the dev stack by lowering
   XI_RATINGS_MAX_AGE_DAYS below the served run's age (env override; the served config
   untouched) and record the observed response in the PR. as_of stays unreachable from
   the product (§ 2.1); do not add it.
4. TESTS. Go: the Result carries both fields (httptest through the handler); a stale
   registry yields the 503 with the code on the wire. Frontend: the date renders on
   success, the refusal renders on 503, neither state is blank. If the Lab shows a new
   labelled number, L-1's completeness gate wants its glossary entry
   (ml-service/ml/xi/glossary.py).
5. DOCS. In § 2.1's gap list mark (1) and (2) closed here with the PR number; (3) and (4)
   stay Phase 2. Update docs/ml-and-training.md's serving section and, if the contract
   moved, make gen-architecture-map.

Acceptance (the P1-5 gate): every successful prediction on the surface carries the date
and run id it was served from, read from its own payload; a stale registry is refused
with RATINGS_STALE and the refusal is shown with the date, never a quietly served number;
make check-all green; coverage gates never move down; cricket_data's three counts
unchanged. Record in docs/PRODUCT_ROADMAP.md § 10 (the P1-5 row). Then stop and hand over
the push and PR commands.
```

#### P1-2 — Play mode: add, remove, swap; every change re-scored live (model: Opus) — **shipped**

*Shipped on `feat/p1-2-play-mode` (2026-09-06). What shipped, what was measured and the
two claims it corrected are in § 10's P1-2 row.*

**What.** The Lab as an instrument: add, remove or swap players on either side, each
change re-scored and its delta shown; constraint chips (keeper, bowlers, must-include) as
visible state. Play mode scores the XI the user built — it does not re-optimise
underneath them — and the displayed probability is the same display model P1-1 shows,
from one code path. Two of the exit gate's clauses are measured here: the swap
monotonicity demonstrated on the surface, and the ~10 ms latency measured end to end
rather than asserted. **Gate:** add/remove/swap on either side re-scores live and shows
the delta; a broken constraint is visible; the probe shows no upgrade lowering the
displayed probability in any format; the latency table is recorded and the ~10 ms claim
is confirmed or corrected in this document.

```
Read docs/PRODUCT_ROADMAP.md § 3 (the Play-mode bullet; the exit gate's clauses 2 and
3), § 3.1 (the 0.0000 swap-violation share and what it cost; the harness's in-process
latency) and § 3.2's P1-2 entry; docs/BUG_BACKLOG.md § B-7 (what "monotone by
construction" means, and the probe that measured it). Read the Lab as P1-1 and P1-5 left
it; go-app's predict path (predict_handlers.go, predictteam/, ml_xi_client.go); and
ml-service's /xi/optimize, /xi/predict-win and /simulate (app/xi_service.py;
app/models/xi.py — XiConstraints carries must_include and must_exclude). Branch off main
as feat/p1-2-play-mode.
Rules: never commit to main; branch off main as the feature branch named above;
conventional commits with scope; anchored edits; make check-all green and coverage
gates never move down — ratchet them up when coverage rises (go-app/Makefile COV_MIN +
root Makefile + .github/workflows/go-app-ci.yml move together; likewise ml-service;
frontend/vite.config.ts thresholds); H-24 for any new wire literal; §8.7 — every
substitution or fallback is visible on the wire, never only in a log. Database rule:
cricket_data holds 22,818 matches, 11,539,808 ball events and 13,662 player_biography
rows — read it freely, run nothing destructive against it; anything destructive goes to
the scratch database cricket_flow_test (make -C go-app test-db;
dbtest.SkipUnlessScratchDatabase), and you verify those three counts unchanged before
you hand over. Checkpoint drill: if usage nears the cap, commit, write RESUME_NOTES.md
with every number already measured, push, and keep going; never report a partial item
as complete.

Do P1-2: Play mode — every change re-scored live, on one code path.
1. THE INTERACTION. On either side: add a player from the pool, remove one, swap one for
   another. Each change re-scores the fixture and shows the delta against the previous
   state: the win probability (headline, with its source), the totals and their ranges,
   the scorecard. Constraint chips — keeper, bowlers (min), must-include — are visible
   state, toggled on the surface and sent as the constraints the API already takes; a
   change that breaks a constraint is shown as broken, never silently repaired.
2. SCORING A HAND-BUILT XI. Play mode scores the XI the user built; it does not
   re-optimise underneath them. Decide, from the code, how a fixed-XI score reaches the
   surface — the existing predict path with both XIs pinned, or the lower calls
   (/xi/predict-win + /simulate) through go-app — and choose the one that keeps ONE code
   path for the numbers: the displayed probability must be the same display model P1-1
   shows, from the same source, and the ranges the same simulator's. Say which and why in
   the PR. An "Optimise" action beside Play mode returns to the searched XI where the
   format is optimised (T20I, ODI); where it is rating-ordered (T20, TEST) it returns the
   rating-ordered pick and the notice says so.
3. MONOTONE BY CONSTRUCTION, DEMONSTRATED. Upgrading a player must never lower the
   displayed probability. Do not re-measure the model (B-7 did, and the served run reads
   0.0000); demonstrate it on the surface with a scripted probe through the real stack:
   for each format, a fixture and a swap of one XI player for a better-rated one from the
   pool, asserting the displayed probability does not fall. A violation here is a
   plumbing bug — wrong model, wrong side, wrong toss state — not a model property; find
   it.
4. LATENCY, MEASURED. The roadmap claims ~10 ms per scenario. Measure what the user sees:
   the end-to-end round trip of one Play-mode re-score, browser -> go-app -> ml-service
   and back, per format, at the served simulator draw count, over enough repeats to quote
   a median and a p95. Record the table in the P1-2 row of docs/PRODUCT_ROADMAP.md § 10.
   If it is not ~10 ms, correct the claim in § 1 and § 3 to the measured figure and say
   where the time goes; do not lower the draw count or add a cache to reach a number.
5. TESTS. Hook tests for the delta arithmetic (pure functions); component tests for the
   chips and the delta rendering; handler tests for whatever the fixed-XI path added.
   Ratchet frontend/vite.config.ts's thresholds up if coverage rose.

Acceptance (the P1-2 gate): add/remove/swap on either side re-scores live and shows the
delta; constraint chips work and a broken constraint is visible; the probe shows no
upgrade lowering the displayed probability in any format; the latency table is recorded
and the ~10 ms claim is either confirmed or corrected in the document; make check-all
green; coverage gates never move down; cricket_data's three counts unchanged. Record in
docs/PRODUCT_ROADMAP.md § 10 (the P1-2 row). Then stop and hand over the push and PR
commands.
```

#### P1-3 — The "why this player" card, from what the objective consumed (model: Opus) — **shipped**

*Shipped on `feat/p1-3-why-this-player` (2026-09-06). What shipped, which four fields were
omitted and why, and what was seen on the dev stack are in § 4 and in § 10's P1-3 row.*

**What.** The card § 4 specifies, on every selected player: marginal value (headline),
role in the XI, expected contribution with its range, form and standing, and the
beats-whom line. The rule is the item, not a footnote: the card shows only inputs the
objective actually consumed, and where selection is rating-ordered it says "picked by
rating, not by the win model" and carries nothing the win model would have said. Anything
the stack cannot produce from a consumed input is omitted with its reason recorded in § 4,
never approximated. **Gate:** the card renders on every selected player in both states;
every field maps to a consumed input named in the PR; the rating-ordered card says so and
carries no marginal value; every number's explainer is reachable.

```
Read docs/PRODUCT_ROADMAP.md § 4 (the card's spec and its honesty rule: only inputs the
objective actually consumed, never post-hoc stories), § 3.1 (which formats are
rating-ordered and why) and § 3.2's P1-3 entry. Read what the response already carries
per player: go-app/internal/services/predictteam/predict_team.go (SelectedPlayer: runs,
wickets and their 10-90 ranges; marginal_value, absent on a rating-ordered XI;
spread_share; SelectionSummary), ml-service/app/xi_service.py (marginal_values; the
spread shares from the simulator) and ml-service/ml/xi/optimizer.py (the constraint
state — which player satisfies the keeper and bowling constraints — and what the
objective reads). Branch off main as feat/p1-3-why-this-player.
Rules: never commit to main; branch off main as the feature branch named above;
conventional commits with scope; anchored edits; make check-all green and coverage
gates never move down — ratchet them up when coverage rises (go-app/Makefile COV_MIN +
root Makefile + .github/workflows/go-app-ci.yml move together; likewise ml-service;
frontend/vite.config.ts thresholds); H-24 for any new wire literal; §8.7 — every
substitution or fallback is visible on the wire, never only in a log. Database rule:
cricket_data holds 22,818 matches, 11,539,808 ball events and 13,662 player_biography
rows — read it freely, run nothing destructive against it; anything destructive goes to
the scratch database cricket_flow_test (make -C go-app test-db;
dbtest.SkipUnlessScratchDatabase), and you verify those three counts unchanged before
you hand over. Checkpoint drill: if usage nears the cap, commit, write RESUME_NOTES.md
with every number already measured, push, and keep going; never report a partial item
as complete.

Do P1-3: the "why this player" card, assembled from what the objective consumed.
1. THE RULE FIRST. Before any UI, write down — in the PR and in the card component's
   doc-comment — the inputs the objective actually read for this selection, from the
   code. The card shows those and only those. Anything the stack cannot produce from a
   consumed input is omitted, with the reason recorded in docs/PRODUCT_ROADMAP.md § 4 —
   not approximated, not fabricated. This is the item, not a footnote.
2. THE FIELDS, each from an existing computation or explicitly new and subject to the
   rule: marginal value (the headline: P(win) lost if replaced by an average player —
   already on the wire); role in the XI (which constraint the player satisfies: keeper, a
   bowling option, must-include — from the optimiser's constraint state, put on the wire
   per player); expected contribution (L2-B's median and 10-90 range for runs and wickets
   in THIS fixture — already on the wire); form and standing (rating percentile within
   the pool as served, computable from the state the objective read; a trajectory only if
   the served state holds a history the objective consumed — otherwise omitted, and § 4
   says so); the beats-whom line (against the best excluded alternative from the same
   pool: the marginal-value gap, with an uncertainty only if the existing computation
   yields one — if it does not, show the gap without one and say the objective gives a
   point estimate here, rather than inventing an interval).
3. THE RATING-ORDERED CASE. Where selection.optimised is false (T20, TEST) the card says
   the simpler truth — "picked by rating, not by the win model" — and shows the rating
   with its pool percentile and the expected contribution with its range, and no marginal
   value or beats-whom line, because nothing was maximised. It must not compute a marginal
   value from the win model for display in a format the policy scoped off.
4. GLOSSARY. Every labelled number on the card is an L-1 key with an entry in
   ml-service/ml/xi/glossary.py, reachable from the card through MetricGlossaryContext;
   the completeness gate will name any that are missing.
5. TESTS. Go and Python tests for anything new on the wire (role, percentile, the gap);
   component tests for the two card states, asserting that the rating-ordered card
   carries no marginal value and no beats-whom line.

Acceptance (the P1-3 gate): the card renders on every selected player in both states;
every field maps to a consumed input named in the PR; the rating-ordered card says so and
carries nothing the win model would have said; every number's explainer is reachable;
make check-all green; coverage gates never move down; cricket_data's three counts
unchanged. Record in docs/PRODUCT_ROADMAP.md § 10 (the P1-3 row) and update § 4 with any
field omitted and why. Then stop and hand over the push and PR commands.
```

#### P1-4 — The honesty surfaces: ranges, the not-optimised notice, the date, the explainers (model: Fable)

**What.** The sweep that makes honesty the UI rather than the footnotes, on every state of
the Lab: every point shown with its 10–90 range, as the simulator served it; the
not-optimised notice at the XI wherever selection is rating-ordered, carrying the served
reason; "ratings as of <date>" from P1-5 on every prediction; every labelled number an L-1
key with its explainer reachable; every failure path honest. Runs last because it checks
what P1-1, P1-5, P1-2 and P1-3 built. **Gate:** an inventory in the PR shows every number
with its range, its explainer and its date; every rating-ordered XI carries the served
reason; every failure state is honest; nothing on the surface adjusts what the stack served.

```
Read docs/PRODUCT_ROADMAP.md § 3 (the honesty bullet; the exit gate's clauses 4 and 6),
§ 3.1 (the rating-ordered formats; the simulator's day/night gap the Lab must not paper
over) and § 3.2's P1-4 entry; docs/ml-and-training.md on L-1 (ml/xi/glossary.py and its
completeness gate) and on §8.7 (a substitution or fallback is on the wire, never only in a
log). Read the Lab as P1-1, P1-5, P1-2 and P1-3 left it;
frontend/src/context/MetricGlossaryContext.tsx; PredictionReadiness.tsx;
ml-service/ml/xi/optimizer.py NOT_OPTIMISED_REASONS; and predictteam's SelectionSummary,
ForecastSummary and WinProbabilitySummary (what the wire says about how each number was
made). Branch off main as feat/p1-4-honesty-surfaces.
Rules: never commit to main; branch off main as the feature branch named above;
conventional commits with scope; anchored edits; make check-all green and coverage
gates never move down — ratchet them up when coverage rises (go-app/Makefile COV_MIN +
root Makefile + .github/workflows/go-app-ci.yml move together; likewise ml-service;
frontend/vite.config.ts thresholds); H-24 for any new wire literal; §8.7 — every
substitution or fallback is visible on the wire, never only in a log. Database rule:
cricket_data holds 22,818 matches, 11,539,808 ball events and 13,662 player_biography
rows — read it freely, run nothing destructive against it; anything destructive goes to
the scratch database cricket_flow_test (make -C go-app test-db;
dbtest.SkipUnlessScratchDatabase), and you verify those three counts unchanged before
you hand over. Checkpoint drill: if usage nears the cap, commit, write RESUME_NOTES.md
with every number already measured, push, and keep going; never report a partial item
as complete.

Do P1-4: the sweep — every honesty surface present, on every state of the Lab.
1. INVENTORY FIRST. Walk the Lab in every state — optimised format, rating-ordered
   format, Play mode after a swap, each toss state, stale ratings, nothing loaded — and
   list in the PR every number shown and what stands beside it. The list is the work
   plan, and the gate is checked against it.
2. RANGES ALWAYS SHOWN. Every point with a 10-90 range on the wire (totals, runs,
   wickets, balls, runs conceded) shows it in every state, never a bare point. The ranges
   are the simulator's as served — no client-side widening or narrowing, and the
   day/night calibration gap X-2 recorded (§ 3.1) is neither adjusted for nor hidden.
3. THE NOT-OPTIMISED NOTICE. Wherever selection.optimised is false, a notice at the XI —
   not a tooltip — says the XI is rating-ordered and why, from selection.note (the served
   reason, never a hardcoded string), and the win probability beside it is labelled as
   the display model's read of that XI, not the result of a search.
4. RATINGS AS OF <DATE>. P1-5's date and run id are on every successful prediction; check
   the refused states still carry theirs. One component renders it everywhere.
5. EXPLAINERS REUSED. Every labelled number on the Lab is an L-1 glossary key whose
   explainer opens from the label through MetricGlossaryContext; add any missing entry in
   ml-service/ml/xi/glossary.py (the completeness gate will name them). The win
   probability's source (display or simulator) and the forecast's source (simulator or
   performance_quantiles) are shown by name, each with its explainer.
6. FAILURE PATHS. The stale (RATINGS_STALE), nothing-loaded, cross-gender-fixture and
   ML-unreachable states each show an honest message with the reason from the wire, and
   never a number. Demonstrate each on the dev stack and say how in the PR.
7. TESTS. Component tests per state asserting that the notice, the ranges, the date and
   the explainer link render; one test that a rating-ordered response never renders
   without the notice.

Acceptance (the P1-4 gate): the inventory in the PR shows every number with its range,
its explainer and its date; every rating-ordered XI carries the served reason; every
failure state is honest; nothing on the surface adjusts what the stack served; make
check-all green; coverage gates never move down; cricket_data's three counts unchanged.
Record in docs/PRODUCT_ROADMAP.md § 10 (the P1-4 row) and, if the inventory found a state
§ 3's gate did not anticipate, add it to the gate. Then stop and hand over the push and PR
commands.
```

## 4. The "why this player" card (Phase 1, and the honesty rule for it)

**Built by P1-3, shipped 2026-09-06.** Per selected player, assembled from existing
computations. The rule: the card shows only inputs the objective actually consumed (H-2's
spirit — explanations are the model's real factors, never post-hoc stories), and where
selection is rating-ordered the card says the simpler truth: picked by rating, not by the
win model.

### What the card shows, and which consumed input each field is

Read from the code as it stands (`ml/xi/optimizer.py`, `app/xi_service.py`,
`go-app/internal/services/predictteam`); the same list is the doc-comment on
`frontend/src/components/WhyThisPlayer.tsx`, so the rule travels with the component.

| field | the consumed input it is |
|---|---|
| **Marginal value** (the headline, searched formats only) | `marginal_values`: P(win) for the eleven minus P(win) with this player's rating vector neutralised, from the objective model the search maximised, against the same opposing eleven |
| **Role in the eleven** | the two constraint predicates `_Pool` evaluates over the served as-of vectors: `is_keeper` (also the objective's `has_keeper` feature) and `is_bowler` (`exp_balls_bowled` against `MIN_BOWLING_BALLS`, also its `n_bowlers` feature) |
| **Expected contribution** | L2-B's median and 10–90 range for runs and wickets in this fixture, already on the wire |
| **Selection rating and its pool percentile** | `rating_order_score`, the one composite `_greedy_seed` orders the pool by — the seed every search starts from, and the whole answer where a format is not searched — with its standing *within the pool as served* and that pool's size |
| **The next best** | the best excluded pool player over exactly the single swaps `_best_neighbour` scores, under the same constraints and against the same opposing eleven, and the P(win) that swap costs |

### What is omitted, and why (the rule applied, not a footnote)

Four fields § 4 first sketched are **not** on the card, because the stack cannot produce
them from an input the objective consumed. Each is omitted rather than approximated:

1. **Trajectory** ("form" as a direction of travel). The served rating state holds decayed
   accumulators as of one date — `RatingState` has no history array and `side_vectors`
   reads one snapshot — so the objective consumed no earlier value of anything. A
   trajectory would have to be computed by re-running the rating pass over a window the
   selection never saw, which is a second opinion about the player, not an explanation of
   the pick. **Form is therefore represented only as it is consumed**: the decayed rates
   inside `selection_rating`, and its standing in the pool.
2. **A "must-include" role.** Still omitted, but the reason changed when B-10 was fixed
   (2026-09-06). It used to be that no such constraint existed on the predict path —
   go-app put a required id into the *pool* (P1-1) and sent ml-service an empty
   `must_include`, so a "required" chip would have named a constraint nothing applied.
   The lock is now real: the ids reach `/xi/optimize` as `must_include`, the seed holds
   them and no swap removes them. The card still does not carry it, for the rule's own
   reason — a lock is the **caller's own input echoed back**, not something the selection
   read *about* the player, and the two roles that are on the card are read off the
   as-of vectors (`is_keeper`, `is_bowling_option`) exactly as the objective reads them.
   Where the ask belongs is per side, and that is where it is reported:
   `selection.must_include` on the answer, and Play mode's constraint chips on a pinned
   eleven (P1-2). The role vocabulary is therefore still two values, `keeper` and
   `bowling_option`, declared in `contracts/ops-console.contract.json` under
   `selection_roles`.
3. **A "top-order anchor" role.** Batting position (`exp_bat_position`) is a
   `PLAYER_ROLE_KEYS` column the *performance* model reads. The selection objective never
   sees it, so it explains the expected contribution and never the pick.
4. **An uncertainty on the beats-whom gap.** The gap is one evaluation of the objective per
   candidate swap and yields a point estimate and nothing else. The card shows the gap and
   says so, rather than inventing an interval. (A gap at or below zero would say the search
   stopped on its evaluation budget rather than at a local optimum; measured on the dev
   stack the minimum was +0.0011.)

### The rating-ordered card, and B-8

Where `selection.optimised` is false (T20, TEST) the card carries **no marginal value and
no next-best line**, because nothing was maximised, and it never computes one from the win
model for a format the policy scoped off. It shows the selection rating with its pool
percentile, the role, and the expected contribution with its range — and one sentence that
is the honest half of `docs/BUG_BACKLOG.md` § B-8: *the ordering on screen is the
selection's own composite and is not the win model's ranking; the two disagree about who is
better, so swapping in a higher-rated player can move the displayed probability down.* This
is where a user learns that before acting on the order. P1-4 carries the other half.

In Play mode there is no card state at all — no `selection_reason` on any player — because
the caller built the eleven and nothing selected it; the card says that and shows only the
expected contribution.

## 5. Phase 2 — data operations for an internal prototype

**Opened 2026-09-07, written for route (a).** Phase 1 closed on functional acceptance
(§ 3): the Lab exists, and every number it serves carries its date and its run. Phase 2 is
the operating layer around it — what keeps the ratings honest between retrains, and what
scores the answers the Lab gives once the matches they were about have been played. The
section as first drafted (2026-09-03) was written for a business: scheduled ingest in
production, a *public* track-record page called "the moat seed", a freshness badge for a
customer's benefit. Route (a) chose an internal prototype with one user (§ 2), and this
section is rewritten for that, not softened: the machinery is kept where it is valuable on
its own terms, and the parts that only a public or a market would pay for are dropped with
the reason recorded. The item-by-item prompts are in § 5.2; the ground truth they build on
is in § 5.1.

**What Phase 2 builds** — the four items of § 5.2, in run order:

- **One freshness verdict** (P2-1, **the priority item**). P0-4's audit (§ 2.1, gap (4))
  found two freshness rules with different inputs and different thresholds coexisting, and
  disagreeing on the day it was written: go-app's `db_freshness` buckets the database's
  latest match per format (`ok` ≤ 7 days, `stale` ≤ 30, `missing` beyond) and read
  **stale** overall because TEST's latest match was 8 days old, while H-11 — 14 days on the
  loaded rating state's `last_date`, the rule a prediction is actually refused on — read
  **fresh** at 2 of 14. Neither number is wrong; they measure different things, and nothing
  says which one a badge means. A system that reports fresh and stale at once will
  eventually be believed at the wrong moment, which is why this runs first. The item is one
  object, assembled once, that every surface reads, with H-11's limit as the only threshold
  and the database's own lag reported as a fact beside it rather than as a second verdict.
- **The run manifest records `ratings_through`** (P2-2). Gap (3) of the same audit: the
  manifest records `cutoff` — the retrain's wall clock, `2026-09-03` on the box — and not
  the date the ratings actually run through (`2026-09-02`), which is read off the loaded
  state at serve time. A run's data date therefore cannot be read from its manifest alone,
  and the listing of runs on disk cannot say which of them is worth loading. Small and
  mechanical, and it removes the one place the as-of date is still not written down.
- **The prediction store** (P2-3). **Issued predictions are not stored today.** A
  prediction is assembled by go-app from four ml-service answers, returned to the Lab, and
  gone: nothing holds what it claimed, so nothing can be scored against the result when
  the match is played. Every successful answer of `POST /api/predict/team-selection` —
  Optimise and Play mode alike — is persisted with what it claimed and the run that served
  it, verbatim. This is the substantive piece of Phase 2 and the prerequisite for the next
  item; it is feasible now, and not before, because P1-5 put `ratings_through` and
  `run_id` on every served prediction.
- **The internal track record** (P2-4). Every stored prediction whose match has since been
  imported is scored against what happened — the win probability by Brier and reliability,
  the served 10–90 totals by coverage — and the cumulative calibration is plotted, **misses
  included**. That honesty is the point of the thing, not a nicety: a record that showed
  only the hits would be the tipster's page § 1 says the harness is the antidote to. It is
  the harness's discipline applied continuously, on the served run, against new data as it
  arrives, rather than once per retrain on folds. It is *internal*: there is no public
  and no moat to seed, and the section says so below.

**What Phase 2 does not build, with the reason recorded.** Kept visible so nothing is lost
silently; none of it is being built:

| dropped or not written | why |
|---|---|
| **The public track-record page** ("the moat seed") | An internal prototype has no public and no moat to seed. What is kept is the machinery — score every issued prediction once its match resolves, plot whether calibration holds on new data — which is valuable to the one user on its own terms. § 1 and § 7 still name a public accuracy record as an asset a business would accumulate; that is what route (b) would build and it stays there as the business case, not as Phase 2 work |
| **Scheduled ingest** (A-5's cadence, in production) | **Ingest stays manual, by the user's choice.** A-5 measured `make cadence` — the `refresh` plan, fetch → extract → import → retrain → reload — at **11 min 22 s** end to end, and `deploy/cadence/` holds a systemd timer and a crontab as documentation, enabled by nothing. The user has chosen to run it by hand, and no scheduling item is written. **The accepted consequence, stated plainly:** H-11 refuses a live prediction against ratings older than `ml.ratings_max_age_days` (14), so the Lab will periodically stop answering until an import → retrain → reload is run. Under the weekly cadence § 2.1 bounded the age at eight or nine days; by hand there is no bound, and the refusal is the mechanism. It is visible rather than silent — P1-5's refusal names the date, the age against the limit and the remedy (`503 RATINGS_STALE`, "ratings run through *date* (*n* days old, limit 14)", the hint naming retrain then reload, and the Lab showing all of it and no number) — so the system fails honestly, and this section says so rather than implying continuous freshness. Nothing here is a claim that the ratings are current; the claim is that they are dated, and refused when the date is too old |
| **A licensed squad/availability feed** | A feed is a paid, account-gated source and the standing constraint rules it out. Availability is **maintained lists**: what the user records, corroborated where held data can corroborate it |
| **A further availability item** | Judged from the code on 2026-09-07 and found **already covered** for one user: D-12 shipped the recency-bounded default pool (`pool.recency_months`, measured per format), the optional manual pool picker (`GET /api/options/candidates`, tick a subset), the retirement ledger (`player_status`, a user's flag promoted to `player.is_retired` only when a criterion corroborates it — inactivity, and since X-1a the Wikidata career-end date and age-with-inactivity, read from `player_biography`), must-include ids that bypass every filter, and every exclusion visible and reversible on the surface; P1-2 added the hand-built eleven. What that leaves uncovered is a durable per-player "unavailable until *date*" note — an injury list — whose only writer and only reader would be the same person, who already has the picker and Play mode for the request in front of them. That is a maintained list with no second maintainer, and it is not written. It becomes an item the day there is a second user, and is recorded here so it is not rediscovered |

**Exit gate — functional acceptance.** These are product and operations items for an
internal prototype, so the gate is functional acceptance in the sense § 3 uses — every
clause is a thing a worker can demonstrate on the dev stack — and not a measured null.
Phase 2 is done when all six hold, and § 10 records it:

1. **One freshness verdict, everywhere.** Every surface that shows freshness — the Health
   tab, Ops Status, the Workbench's loaded-run card, the system map, the Lab's readiness
   notice and its `RATINGS_STALE` refusal — reads one object assembled once, with H-11's
   `ml.ratings_max_age_days` as the only threshold; the database's per-format lag is on
   the same object as a fact, not as a second verdict. P0-4's disagreement is reproduced on
   the dev stack (a sparse format days past the old 7-day bucket while the served ratings
   are inside the limit) and the reconciled surfaces all read *fresh*, with the format's
   lag stated beside it; with the limit lowered under the served age, the badge, the
   readiness notice and the prediction refuse together, naming the same date.
2. **A run's data date is readable without loading it.** Every manifest written by
   `make retrain` carries `ratings_through` beside `cutoff`; the runs listing shows it for
   every run on disk; the loaded run's manifest date equals the `ratings_through` every
   served prediction carries, and a run whose manifest and state disagree — or whose
   manifest lacks the field — is refused by name rather than served.
3. **Every issued prediction is stored, verbatim.** Each successful
   `POST /api/predict/team-selection` answer is persisted with the payload the Lab
   received, byte-for-byte as JSON, the request it answered, and the `run_id` and
   `ratings_through` it was served from; reading a stored prediction back reproduces the
   served payload exactly. A store failure is on the wire of the answer it failed to
   record, never only in a log, and never refuses the prediction.
4. **Resolution needs no operator step.** After an import brings the match a stored
   prediction was about, the record shows it resolved and scored — computed on read from
   the store and the `match` tables, with no new pipeline step and no scheduler.
5. **Misses are on the record.** The record shows Brier and the reliability curve over
   *every* scored prediction with the base-rate Brier beside them, the served 10–90
   coverage per innings, and — visibly counted, never dropped — the predictions that are
   unresolved, no-result, superseded by a later forecast of the same fixture, or scenarios
   (hand-built elevens) that are listed and not scored. Predictions served by a simulator
   without a shared factor are reported as their own population, never pooled (B-12).
   Nothing on the surface is filtered by outcome.
6. **Manual ingest fails honestly.** With the ratings past the limit, the Lab refuses
   with the date, the age against the limit and the remedy; the freshness verdict and the
   track record show the same `ratings_through`; and after an import → retrain → reload
   run by hand, the new run is served, the verdict is fresh, and the record scores under
   the new run without losing what the old one issued.

What this gate does not measure, said so nobody reads it as more: **whether calibration
holds.** The record will hold tens of predictions over the weeks one user issues them, and
a Brier or a reliability bin over tens is a diagnostic, not evidence — every number on it
is shown with its *n*, no threshold is set on it, and `make evaluate` remains where a
choice-facing number comes from. A record that disagrees with the harness is a thing to
investigate and write into `docs/BUG_BACKLOG.md`, not a verdict on the model. And, as
Phase 1's gate said of itself: nothing here is a claim about users or a market — retention,
pull and cost per user stay unmeasured under route (a).

### 5.1 What Phase 2 inherits

Ground truth the four items build on, recorded here so no worker rediscovers it:

- **The Lab exists, and every served prediction is dated and attributed** (P1-5, PR #270).
  `predictteam.Result` embeds `ServedRatings {run_id, ratings_through}`, required fields,
  stamped by ml-service on each of the answers a prediction is assembled from
  (`/xi/optimize`, `/xi/predict-win`, `/simulate`, `/performance/predict` each carry
  `served_ratings`, read off the store that computed the answer — `manifest.run_id` and
  `state.last_date`); go-app requires every stamp to agree and refuses a prediction that
  straddled a reload with `409 SERVED_RUN_CHANGED`. **This is what makes a prediction store
  feasible:** a stored answer can name the run that produced it without a status read that
  might describe a different run. The payload also names every substitution it made —
  `selection.objective` (`win`, `ratings` or `fixed`), `optimised` and `note`,
  `forecast.source`, `win_probability.source`, `toss`, both pool summaries — so a stored
  prediction is self-describing (§8.7).
- **A prediction store existed once and was dropped, and this is not its revival.**
  Migration `0007` dropped `match_prediction_aggregates`, a cache the per-match backtest
  flow wrote for a played match and read back for an accuracy trend, together with the
  five models that filled it. P2-3 stores *issued* answers about *upcoming* fixtures, with
  the run that served them; the thing it records cannot be reproduced from the artifacts
  after the run is replaced, which is the opposite of a cache.
- **The freshness machinery, as it stands** (read from the code 2026-09-07; § 2.1 has the
  full audit). ml-service: `XiRegistry.freshness()` (`app/xi_service.py`) is H-11's verdict
  — `RatingsFreshness {fresh, age_days, max_age_days, ratings_through, code}` against
  `ml.ratings_max_age_days` — and the same computation refuses a live request past the
  limit; `/xi/status` and `/health` carry it. go-app: `BuildDBFreshnessSection`
  (`internal/services/opsstatus/db_insights.go`) buckets the latest match per format at
  7 and 30 days and rolls a worst-of `overall` onto `/ops/status` as `db_freshness`, beside
  `artifacts` (ml-service's `/artifacts/status` copied through whole) and a separate
  `db_completeness` (matches in the last 30 days per format — a different question, "is
  the import empty?", and not part of the disagreement). Frontend: `HealthTab.tsx`
  renders the H-11 line ("through *date* (*n* days old)"), `OpsStatusDetailsGrid.tsx`
  renders the `db_freshness` grid through `opsStatusHelpers.readStatus`'s
  `ok | stale | missing | unknown`, `WorkbenchRunSection.tsx` the loaded run,
  `PredictionReadiness.tsx` and `RatingsAsOf.tsx` the Lab's states, and
  `systemMap/bindings` the `ratings_through` / `ratings_age` bindings. Two thresholds,
  three surfaces of vocabulary, one rule that refuses.
- **The manifest and the state.** `RunManifest` (`ml/xi/runs.py`) carries `run_id`,
  `created_at`, `cutoff`, `dataset_sha`, `git_sha`, `rating_params`, `hyperparameters`,
  `metrics`, `state_shape`, `formats`, `format_notes`, `report` — and no date the ratings
  run through. `retrain.py` builds it with `result.state.last_date` in hand (it already
  reads the state for `state_shape`), and `store.py` saves `last_date` inside the
  `xi_ratings.joblib` payload, which is the only place it is written. The loader
  (`XiStore.load`) refuses a run with no manifest or the wrong arrays as
  `RunArtifactsInvalid`, naming the run (D-6); `/artifacts/status` lists every run on disk
  from `runs.list_runs` with `current` and `loaded` flags, and go-app copies the listing
  through. The manifest is written last, so a directory is a run only once it exists.
- **What the database holds for scoring.** `match` carries `match_date`, `format_id`,
  `gender`, `venue_id`, `outcome_winner_opposition_id` (NULL for a no-result),
  `outcome_by_runs` / `outcome_by_wickets`; `match_inning` carries `runs_scored` and
  `winner_opposition_id` per innings; `match_player` carries the fielded elevens, which
  are what the harness and E5 build sides from. A prediction names its fixture by the two
  opposition ids, the format, the gender and `match_date`; the import that brings the
  played match brings the join key with it.
- **The harness's scoring functions are reusable as they are.** `ml/xi/sim_harness.py`
  has `reliability()` (mean predicted against observed per equal-width bin,
  `RELIABILITY_BINS` = 10, with `n` per bin) and the Brier; the glossary already carries a
  `reliability` entry (L-1). Coverage of a 10–90 range is a comparison, not a model. What
  is *not* on the serving wire: whether the served simulator for a format carries a shared
  factor — the fitted calibration knows (`SimulatorCalibration.as_dict` reports
  `shared_factor: null` when the calibration fold was too thin to fit one), and
  `SimulateResponse` does not say.
- **D-12's ledger is the first piece of maintained availability, and it is wired to
  X-1a.** `internal/availability` evaluates three pluggable criteria at promotion time —
  inactivity (5 years, measured), the Wikidata career-end date and age with inactivity,
  the last two reading `player_biography` and reporting *unavailable* for a player it
  holds no facts about. A flag alone excludes the player from that user's default pools;
  only a corroborated flag becomes `player.is_retired`. See the table above for why no
  further availability item is written.
- **B-11 is open** (`docs/BUG_BACKLOG.md`; plan §8.13): the simulator fits one dispersion
  to two populations, and its 10–90 interval is wrong in opposite directions on each —
  T20 first-innings coverage **0.734 by day against 0.841 at night** at a nominal 0.80.
  Both gated candidates (`SIM-DN-split`, `SIM-DN-scale`) recorded nulls: each fixes the
  day side, overshoots the night side, and makes the night chase worse by five standard
  errors, because the factor is shared by the two innings and at night they want opposite
  corrections. The next candidate is a dispersion term the two innings do not share. For
  Phase 2 this means the track record's coverage numbers will show the miss, and must:
  no client-side widening, no day/night adjustment, no hiding of a range (§ 3.1's rule
  for the Lab holds for the record).
- **B-12 is fixed, and it changes how a record may pool.** A fold whose calibration
  window was too thin to fit a shared factor used to be averaged into the harness's
  simulator totals with nothing saying so; `sim_harness.summarize_folds` now reports
  `shared_factor_folds` — how many folds had a factor, which windows did not, and the
  same totals over the calibrated folds alone — and the Evaluation tab carries it. The
  served run is subject to the same thinness: `fit_performance` ships a format's
  simulator without a factor when its calibration fold is short, with a warning. **A
  track record must not re-pool them**: a prediction served by a factorless simulator is a
  different population from one served with a factor, and the record reports the two
  separately with both denominators visible, as the harness now does.

### 5.2 Phase 2 kickoff prompts

One prompt per item, in the [EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) style: run each
in a fresh chat with the model noted, and each ends by handing over the push and PR
commands. Run order: **P2-1 → P2-2 → P2-3 → P2-4** — the freshness verdict first because
it is the priority and depends on nothing, the manifest next because it is small and
every later surface can then read a run's date off disk, then the store, and last the
record that reads it. These are product and operations items, so each gate is functional
acceptance, not a measured null. Every prompt carries the same rules in its own words —
branch off `main` as a named feature branch and never commit to `main`; conventional
commits with scope; anchored edits; `make check-all` green and coverage gates never moving
down (go-app `COV_MIN` is **77**, ml-service **93**, frontend as set in
`frontend/vite.config.ts`; a ratchet is verified against the CI run's own figure, per
B-9); H-24 for any new wire literal; §8.7; the database rule; the checkpoint drill; and
one operational trap — `make evaluate` takes 54–70 minutes and a foreground run was killed
at ~55 minutes by a background-task reaper, minutes before the harness writes its JSON,
losing the whole run, so any harness run is launched detached and waited on — so that a
fresh worker needs nothing but the prompt.

#### P2-1 — One freshness verdict: `db_freshness` reconciled with H-11 (model: Opus) — **priority**, **shipped**

**What.** Close gap (4) of P0-4's audit (§ 2.1). Two freshness rules coexist with different
inputs and thresholds — go-app's `db_freshness` (the database's latest match per format,
bucketed at 7 and 30 days, worst-of overall) and H-11 (14 days on the loaded rating
state, the rule that refuses) — and on 2026-09-04 they disagreed: *stale* because TEST's
latest match was 8 days old, *fresh* at 2 of 14. The item replaces them with **one
freshness object, assembled once in go-app, that every surface reads**: H-11's verdict as
ml-service gives it (the only threshold and the only badge), the database's per-format lag
as a dated fact beside it, and whether the database holds matches the served run never
saw (the B-2 state, "retrain due") as a third fact. The 7/30 buckets go. **Gate:** every
freshness surface reads the one object; H-11's limit is the only threshold in the system;
P0-4's disagreement, reproduced on the dev stack, reads as one verdict with the format's
lag stated beside it; lowering the limit makes the badge, the readiness notice and the
prediction refuse together, naming the same date.

```
Read docs/PRODUCT_ROADMAP.md § 2.1 (P0-4's audit — gap (4) is this item's; the machinery
list is the inventory of what you are reconciling), § 5 (Phase 2's gate, clause 1 and 6),
§ 5.1 (the freshness machinery as it stands) and § 5.2's P2-1 entry. Read the code:
ml-service/app/xi_service.py (XiRegistry.freshness — H-11's verdict; RatingsStale -> 503
RATINGS_STALE), ml-service/app/models/xi.py (RatingsFreshness), ml-service/config.default.json
(ml.ratings_max_age_days, 14; XI_RATINGS_MAX_AGE_DAYS overrides; 0 turns it off);
go-app/internal/services/opsstatus/db_insights.go (BuildDBFreshnessSection: the 7/30
buckets and the worst-of overall; BuildDBCompletenessSection beside it, a different
question), assemble.go and types.go (db_freshness and artifacts on /ops/status),
artifacts.go (ml-service's /artifacts/status copied through); frontend/src/components/
HealthTab.tsx (the Ratings line), OpsStatusDetailsGrid.tsx and
utils/opsStatusHelpers.ts (readStatus: ok | stale | missing | unknown),
WorkbenchRunSection.tsx, PredictionReadiness.tsx, RatingsAsOf.tsx,
systemMap/bindings.ts; contracts/ops-console.contract.json (H-24 vocabularies) and
contracts/system-map.json with scripts/check-system-map.py (in make check-all). Branch
off main as feat/p2-1-one-freshness-verdict.
Rules: never commit to main; branch off main as the feature branch named above;
conventional commits with scope; anchored edits; make check-all green and coverage
gates never move down — ratchet them up when coverage rises (go-app/Makefile COV_MIN,
now 77, + root Makefile COV_MIN_GO + .github/workflows/go-app-ci.yml move together;
ml-service likewise at 93; frontend/vite.config.ts thresholds) and verify a ratchet
against the CI run's own coverage figure, never a local one (B-9: the same commit has
measured 77.0 locally and 76.9 in CI); H-24 for any new wire literal — a freshness code
or status word either side matches on is declared once in
contracts/ops-console.contract.json and asserted from both sides; §8.7 — every
substitution or fallback is visible on the wire, never only in a log. Database rule:
cricket_data holds 22,818 matches, 11,539,808 ball events and 13,662 player_biography
rows — read it freely, run nothing destructive against it; anything destructive goes to
the scratch database cricket_flow_test (make -C go-app test-db, which runs -p 1
deliberately; dbtest.SkipUnlessScratchDatabase), and you verify those three counts
unchanged before you hand over. Checkpoint drill: if usage nears the cap, commit, write
RESUME_NOTES.md in the worktree with every number already measured (do not commit it),
push early, and keep going; never report a partial item as complete. Harness trap: this
item should not need make evaluate; if you do run it, it takes 54-70 minutes and a
foreground run was killed at ~55 minutes by a background-task reaper before the report
was written — launch it detached (nohup, output to a file) and poll for the JSON.

Do P2-1: one freshness verdict, assembled once, read everywhere.
1. THE OBJECT. go-app is the one component that sees both the database and ml-service,
   so it assembles a single `freshness` object for /ops/status with three named facts
   and nothing else: (a) `served` — H-11's verdict exactly as ml-service reports it
   (ratings_through, age_days, max_age_days, fresh, code), copied through and never
   recomputed with a second threshold — this is the only thing that decides "a
   prediction would be refused" and the only overall badge; (b) `database` — per
   format, the latest match date and its age in days, as facts with no bucket of their
   own: the 7/30 buckets answered a question nobody had defined, and `stale` at 8 days
   on a format that plays a Test a fortnight apart is not a fault; (c) `retrain_due` —
   whether the database's latest match date is after `served.ratings_through`, with the
   number of days, because matches imported that the served run never saw is exactly the
   B-2 state a green pipeline once hid. `db_freshness` and its `overall` are deleted, not
   kept beside the new object (the project is not live; two objects is the defect).
   `db_completeness` asks a different question (is the import empty?) and is left as it
   is unless you can say why it belongs in the object. One threshold in the whole
   system: ml.ratings_max_age_days, read from ml-service's verdict. If go-app must know
   the limit for any reason, it reads it off the verdict, never from its own config.
2. THE VOCABULARY. The status words and codes the surfaces render (fresh, stale, not
   loaded, retrain due, and whatever else you find you need) are one vocabulary declared
   in contracts/ops-console.contract.json under H-24, asserted from go-app and the
   frontend (and ml-service, where RATINGS_STALE originates); readStatus's private
   ok | stale | missing | unknown goes with the buckets it read.
3. EVERY SURFACE READS IT. The Health tab's Ratings line, the Ops Status grid (the
   per-format lag as dates and days, the served verdict as the badge, retrain-due as its
   own line), the Workbench's loaded-run card and the system map's ratings bindings all
   read the one object. The Lab's PredictionReadiness and its RATINGS_STALE refusal read
   ml-service's verdict through /api/ml/xi-status and the prediction itself — that is the
   same computation, and it must stay so: add a test that the verdict on /ops/status and
   the verdict on /api/ml/xi-status describe the same store identically. Update
   contracts/system-map.json for anything the map binds.
4. REPRODUCE THE DISAGREEMENT, THEN SHOW IT GONE. On the dev stack, produce the P0-4
   state — a sparse format whose latest match is past 7 days while the served ratings are
   inside 14 (TEST usually is; if not, name the format you used) — and record what the
   old surfaces read (stale / fresh) and what the new ones read (one verdict, the lag as
   a fact). Then, with XI_RATINGS_MAX_AGE_DAYS lowered under the served age as an env
   override on a branch ml-service (the served config untouched), show the badge, the
   readiness notice and POST /api/predict/team-selection refusing together and naming the
   same date. Record both observations in the PR with the run id and the dates.
5. TESTS AND DOCS. Go: the object's three facts from a fake probe and a fake ml-service
   status (httptest), retrain-due true and false, a nothing-loaded state, and the
   contract vocabulary asserted; frontend: each surface renders the one object, the
   readiness notice and the Ops badge agree for the same input, and the contract
   vocabulary asserted; ml-service: RATINGS_STALE asserted against the contract. Docs:
   docs/PRODUCT_ROADMAP.md § 2.1 marks gap (4) closed with the PR number; docs/
   observability.md and docs/apis-backtest-and-ops.md describe the object; README's
   RATINGS_STALE line still true; make gen-architecture-map if a contract moved.

Acceptance (the P2-1 gate): every freshness surface reads one object assembled once;
H-11's limit is the only threshold in the system and go-app holds no copy of it;
P0-4's disagreement reproduced on the dev stack reads as one verdict with the format's
lag stated beside it; lowering the limit makes the badge, the readiness notice and the
prediction refuse together naming the same date; make check-all green; coverage gates
never move down; cricket_data's three counts unchanged. Record what shipped and both
observations in docs/PRODUCT_ROADMAP.md § 10 (the P2-1 row). Then stop and hand over the
push and PR commands.
```

#### P2-2 — The run manifest records `ratings_through` (model: Fable)

**What.** Close gap (3) of P0-4's audit (§ 2.1). `RunManifest` records `cutoff` — the
retrain's wall clock — and not the date the ratings run through, which lives only inside
`xi_ratings.joblib` as `state.last_date` and is read at serve time. So "what date is this
run's data?" cannot be answered from the manifest, and the listing of runs on disk cannot
say which run is worth loading. The manifest gains `ratings_through`, written by retrain
from the state it just built; the loader asserts the manifest and the state agree and
refuses a run that does not carry the field or disagrees with itself; the runs listing and
the surfaces that show runs carry the date for every run, not only the loaded one.
**Gate:** every manifest `make retrain` writes carries `ratings_through` beside `cutoff`;
the runs listing shows it per run; the loaded run's manifest date equals the
`ratings_through` on every served prediction; a manifest without the field, or one whose
date disagrees with its state, is refused by name.

```
Read docs/PRODUCT_ROADMAP.md § 2.1 (P0-4's audit — gap (3) is this item's), § 5 (Phase 2's
gate, clause 2), § 5.1 (the manifest and the state) and § 5.2's P2-2 entry;
docs/ml-and-training.md § Runs, manifests and staleness. Read the code:
ml-service/ml/xi/runs.py (RunManifest, summary(), write_manifest, list_runs),
ml-service/ml/xi/retrain.py (where the manifest is built — result.state.last_date is
already in hand for state_shape), ml-service/ml/xi/store.py (last_date saved in the
joblib payload; XiStore.load and RunArtifactsInvalid, D-6), ml-service/app/xi_service.py
(XiRegistry.status and _served_ratings — the two places ratings_through is read off the
state today), ml-service/app/main.py (/artifacts/status: runs.list_runs with current and
loaded flags), go-app/internal/services/opsstatus/artifacts.go (the listing copied
through), frontend/src/components/OpsRunsPanel.tsx and WorkbenchRunSection.tsx. Branch
off main as feat/p2-2-manifest-ratings-through.
Rules: never commit to main; branch off main as the feature branch named above;
conventional commits with scope; anchored edits; make check-all green and coverage
gates never move down — ratchet them up when coverage rises (go-app/Makefile COV_MIN,
now 77, + root Makefile COV_MIN_GO + .github/workflows/go-app-ci.yml move together;
ml-service likewise at 93; frontend/vite.config.ts thresholds) and verify a ratchet
against the CI run's own coverage figure, never a local one (B-9); H-24 for any new wire
literal; §8.7 — every substitution or fallback is visible on the wire, never only in a
log: a manifest without the date is refused with the reason, never served with a date
read from somewhere else. Database rule: cricket_data holds 22,818 matches, 11,539,808
ball events and 13,662 player_biography rows — read it freely, run nothing destructive
against it; anything destructive goes to the scratch database cricket_flow_test (make -C
go-app test-db, -p 1 deliberately; dbtest.SkipUnlessScratchDatabase), and you verify
those three counts unchanged before you hand over. Checkpoint drill: if usage nears the
cap, commit, write RESUME_NOTES.md in the worktree with every number already measured
(do not commit it), push early, and keep going; never report a partial item as complete.
Harness trap: this item is verified by a retrain (~12 minutes), not by make evaluate; if
you do run the harness, it takes 54-70 minutes and a foreground run was killed at ~55
minutes by a background-task reaper before it wrote its JSON — launch it detached
(nohup, output to a file) and poll for the report.

Do P2-2: the manifest says what date its data runs through.
1. THE FIELD. RunManifest gains ratings_through (YYYY-MM-DD, the rating state's
   last_date), required; retrain writes it from result.state.last_date beside cutoff,
   and summary() carries it so /xi/status reports the manifest's value. Say in the
   manifest's docstring why cutoff and ratings_through are different dates (the cutoff is
   the training boundary the operator asked for — today, for a refresh — and
   ratings_through is the last match the pass actually consumed; on the dev box they
   differed by a day on the served run).
2. ONE DATE, ASSERTED. The loader (XiStore.load) refuses, as RunArtifactsInvalid naming
   the run and both dates, a run whose manifest ratings_through disagrees with the
   state's last_date, and refuses one whose manifest lacks the field: the project is not
   live and nothing is backfilled — an older run is retrained, not patched. _served_ratings
   and status() keep reading the state (that is the store that computed the answer) and
   the assertion at load is what makes the manifest's date the same date; add a test
   for each refusal and one that a loaded run's status, its served_ratings stamp and its
   manifest agree.
3. THE LISTING. /artifacts/status carries ratings_through per run from list_runs, so
   the question "what date is this run's data?" is answered off the listing for every
   run on disk; go-app copies it through; OpsRunsPanel and the Workbench's run section
   show "ratings through <date>" beside the cutoff for every run, and the loaded one
   reads the same date the Lab shows on a prediction. A run on disk whose manifest
   predates this field is listed with the reason it cannot be loaded (§8.7), not with a
   blank.
4. VERIFY on the database: make retrain (record the run id and the ~12 minutes), make
   reload, then read the manifest, /artifacts/status, /xi/status and one
   POST /api/predict/team-selection and record the four dates in the PR — they are one
   date. Then show the refusal: copy the run, remove ratings_through from the copy's
   manifest, POST /admin/reload?run=<copy> and record the 409 naming the run. The
   previously served run, written before this field, will be refused after this lands —
   say so in the PR and leave the new run serving.
5. DOCS. docs/ml-and-training.md § Runs, manifests and staleness (the manifest's field
   list and the load-time assertion); docs/PRODUCT_ROADMAP.md § 2.1 marks gap (3) closed
   with the PR number; make gen-architecture-map if the status contract moved.

Acceptance (the P2-2 gate): every manifest make retrain writes carries ratings_through
beside cutoff; the runs listing shows it per run; the loaded run's manifest date equals
the ratings_through on a served prediction; a manifest without the field, or one whose
date disagrees with its state, is refused by name; make check-all green; coverage gates
never move down; cricket_data's three counts unchanged. Record in
docs/PRODUCT_ROADMAP.md § 10 (the P2-2 row). Then stop and hand over the push and PR
commands.
```

#### P2-3 — The prediction store: every issued prediction persisted, verbatim (model: Opus)

**What.** Issued predictions are not stored today: a `POST /api/predict/team-selection`
answer is assembled, returned and gone, so nothing can be scored against a result later.
Every successful answer — Optimise and Play mode, searched, rating-ordered and hand-built
alike — is persisted in go-app with the payload the Lab received (verbatim JSON), the
request it answered, when it was issued, and the `run_id` and `ratings_through` it was
served from, plus the columns a resolver will join on. A store failure is on the wire of
the answer it failed to record and never refuses the prediction. This is the prerequisite
for P2-4 and the substantive piece of the phase. **Gate:** every successful prediction on
the dev stack is stored and reads back byte-equal to what was served, with its run and
date; a refused prediction stores nothing; a store failure is named on the wire; the
harness and the pipeline are untouched.

```
Read docs/PRODUCT_ROADMAP.md § 5 (what Phase 2 builds; the gate's clause 3), § 5.1 (what
every served prediction already carries; the dropped table this is not; what the
database holds for scoring) and § 5.2's P2-3 entry; docs/EXTERNAL_DATA_PLAN.md § D-12
and its record (the ledger's migration 0009_player_status.sql is the precedent for a
go-app store with a forward migration and a history). Read the code:
go-app/internal/server/predict_handlers.go (predictTeamRequest and the handler; the
fixture is team1_id/team2_id, the genders, venue, match_date; team1_xi/team2_xi pin an
eleven; team1_bats_first), go-app/internal/services/predictteam/predict_team.go
(Result: ServedRatings, both sides, selection, forecast, win_probability, toss,
scorecard, pools, constraints), served_ratings.go, play_mode.go; go-app/migrations/
(0012 is the latest; 0007 dropped match_prediction_aggregates and says why);
go-app/internal/db/repo_player_status.go (a store the way this repo writes one) and
dbtest/; go-app/internal/mocks (mockery from contract.go). Branch off main as
feat/p2-3-prediction-store.
Rules: never commit to main; branch off main as the feature branch named above;
conventional commits with scope; anchored edits; make check-all green and coverage
gates never move down — ratchet them up when coverage rises (go-app/Makefile COV_MIN,
now 77, + root Makefile COV_MIN_GO + .github/workflows/go-app-ci.yml move together;
ml-service likewise at 93; frontend/vite.config.ts thresholds) and verify a ratchet
against the CI run's own coverage figure, never a local one (B-9); H-24 for any new wire
literal — a record status either side matches on is declared in
contracts/ops-console.contract.json and asserted from both; §8.7 — every substitution
or fallback is visible on the wire, never only in a log: a store failure is reported on
the prediction it failed to record. Database rule: cricket_data holds 22,818 matches,
11,539,808 ball events and 13,662 player_biography rows — read it freely, run nothing
destructive against it; the new table is a forward migration and the only thing this
item writes there; anything destructive goes to the scratch database cricket_flow_test
(make -C go-app test-db, -p 1 deliberately; dbtest.SkipUnlessScratchDatabase), and you
verify those three counts unchanged before you hand over. Checkpoint drill: if usage
nears the cap, commit, write RESUME_NOTES.md in the worktree with every number already
measured (do not commit it), push early, and keep going; never report a partial item as
complete. Harness trap: this item must not need make evaluate; if you run it to prove
blast radius, it takes 54-70 minutes and a foreground run was killed at ~55 minutes by
a background-task reaper before it wrote its JSON — launch it detached (nohup, output
to a file) and poll for the report.

Do P2-3: the prediction store — every issued answer, kept as it was served.
1. THE TABLE. A forward migration (0013) creates the store in go-app's database: one row
   per successful answer, holding the served payload verbatim as jsonb (so a later scorer
   never lacks a field and the record can show exactly what was claimed), the request
   as jsonb, issued_at, run_id and ratings_through as columns, and the columns a resolver
   joins on and a record sorts by — format code, both opposition ids, gender,
   match_date, the selection objective (win | ratings | fixed), and the headline
   win probability with its source. Nothing derived is stored that the payload does not
   already say. Say in the migration's comment what this is and what 0007's dropped
   table was, so nobody reads it as a revival of a cache.
2. THE WRITE. After predictteam.Predict succeeds and before the handler answers, the
   answer is recorded through a small interface (one method; mockery mock in
   internal/mocks) the handler depends on. Every successful answer is recorded — Play
   mode re-scores included, because the record is what makes P2-4's counting honest —
   and a refused one is not: a refusal is not a prediction. The response gains a
   `record` block naming the stored row's id and issued_at; when the store fails, the
   block says `stored: false` with the reason and the prediction is still served —
   decide that rule from the product (the Lab is the product; the record is the record)
   and say why in the commit. Never only a log line.
3. THE READ. GET /api/predictions/{id} returns a stored answer; GET /api/predictions
   lists them newest first with the join columns, paged. Reading a row back yields the
   served payload byte-equal as JSON — assert it through the real handler against the
   scratch database, not by comparing structs.
4. BLAST RADIUS. Nothing under ml-service/ changes; the harness, the run plans and the
   three pipeline steps are untouched (grep for readers of the new table: there must be
   only this item's). The write is one insert per prediction; measure what it adds to
   P1-2's 355-399 ms re-score on the dev stack and record it in the PR.
5. VERIFY the D-6 way on the dev stack: an Optimise in T20I, a rating-ordered T20, a
   Play-mode re-score, and a refused prediction (XI_RATINGS_MAX_AGE_DAYS lowered on a
   branch ml-service); read the three rows back and diff them against the served
   bodies; confirm the refusal wrote nothing; then point the store at an unreachable
   database for one request and record the `record.stored: false` block on the wire.
6. TESTS AND DOCS. Go: the handler records on success and not on refusal (mock), the
   failure block on the wire, the read endpoints (httptest), and scratch-database
   integration tests for the round trip and the byte-equality; frontend: if the Lab
   shows the record id beside the date chip, a component test for it. Docs:
   docs/apis-backtest-and-ops.md (the two endpoints), docs/config-and-data.md (the
   table), make gen-architecture-map if a contract moved.

Acceptance (the P2-3 gate): every successful prediction on the dev stack is stored and
reads back byte-equal to what was served, with its run and date; a refused prediction
stores nothing; a store failure is named on the wire and does not refuse the prediction;
nothing in ml-service, the harness or the pipeline changed; make check-all green;
coverage gates never move down; cricket_data's three counts unchanged. Record in
docs/PRODUCT_ROADMAP.md § 10 (the P2-3 row). Then stop and hand over the push and PR
commands.
```

#### P2-4 — The internal track record: resolved predictions scored, misses included (model: Fable)

**What.** The stored predictions (P2-3) scored against what happened, computed on read
from the store and the `match` tables — no new pipeline step, no scheduler. Per fixture
one forecast is scored, the rest are counted; the win probability is scored by Brier and
reliability with the base rate beside it, the served 10–90 totals by coverage per innings;
predictions served by a factorless simulator are their own population (B-12); the
predicted elevens are compared with the fielded ones and the overlap shown. Every state a
prediction can be in — scored, unresolved, no-result, superseded, scenario — is counted on
the surface, and nothing is filtered by outcome. It is internal, and it is not a gate: *n*
is shown on every number and no threshold is set. **Gate:** the record on the dev stack
shows every stored prediction in exactly one named state; the scored ones carry Brier,
reliability and coverage with their *n*; an import that brings a predicted match moves it
to scored with no operator step; a miss is on the record; the two simulator populations
are never pooled.

```
Read docs/PRODUCT_ROADMAP.md § 5 (what the record is and is not; the gate's clauses 4, 5
and 6; what the gate does not measure), § 5.1 (what the database holds for scoring; the
harness's reusable scoring; B-11 open; B-12 and why a record must not re-pool) and
§ 5.2's P2-4 entry; docs/BUG_BACKLOG.md § B-11 and § B-12; docs/ml-and-training.md
§ Evaluation harness. Read the code: the store P2-3 shipped (its migration, repo and
endpoints); go-app/internal/db/repo_match.go and migrations/0001_baseline.sql (match:
match_date, format_id, gender, outcome_winner_opposition_id NULL for a no-result,
outcome_by_runs/wickets; match_inning: runs_scored; match_player: the fielded elevens);
ml-service/ml/xi/sim_harness.py (reliability(), RELIABILITY_BINS = 10, _brier;
summarize_folds and shared_factor_folds), ml-service/ml/xi/simulator.py
(SimulatorCalibration.as_dict: shared_factor null when none was fitted),
ml-service/app/models/xi.py (SimulateResponse — carries no calibration flag today),
ml-service/ml/xi/glossary.py (L-1; `reliability` exists; the completeness gate);
frontend/src/components/EvaluationReportTab.tsx and its sections (how the harness's
numbers are shown, and the B-12 chip), App.tsx (the tabs). Branch off main as
feat/p2-4-internal-track-record.
Rules: never commit to main; branch off main as the feature branch named above;
conventional commits with scope; anchored edits; make check-all green and coverage
gates never move down — ratchet them up when coverage rises (go-app/Makefile COV_MIN,
now 77, + root Makefile COV_MIN_GO + .github/workflows/go-app-ci.yml move together;
ml-service likewise at 93; frontend/vite.config.ts thresholds) and verify a ratchet
against the CI run's own coverage figure, never a local one (B-9); H-24 for any new wire
literal — the prediction states are a vocabulary declared in
contracts/ops-console.contract.json and asserted from go-app and the frontend; §8.7 —
every substitution or fallback is visible on the wire, never only in a log. Database
rule: cricket_data holds 22,818 matches, 11,539,808 ball events and 13,662
player_biography rows — read it freely, run nothing destructive against it; anything
destructive goes to the scratch database cricket_flow_test (make -C go-app test-db, -p 1
deliberately; dbtest.SkipUnlessScratchDatabase), and you verify those three counts
unchanged before you hand over. Checkpoint drill: if usage nears the cap, commit, write
RESUME_NOTES.md in the worktree with every number already measured (do not commit it),
push early, and keep going; never report a partial item as complete. Harness trap: the
record reads the existing xi_evaluate_report.json for comparison and should not need a
new one; if you do run make evaluate, it takes 54-70 minutes and a foreground run was
killed at ~55 minutes by a background-task reaper minutes before it wrote its JSON,
losing the run — launch it detached (nohup, output to a file) and poll for the report.

Do P2-4: the internal track record, computed on read, misses included.
1. THE STATES. Every stored prediction is in exactly one state, and the vocabulary is on
   the wire (H-24): SCENARIO — a hand-built eleven (selection.objective fixed), listed and
   never scored, because the caller built a side that may not have played; SUPERSEDED —
   an Optimise forecast of a fixture that a later Optimise of the same fixture (same
   format, opposition pair, gender, match_date) issued before the match date replaced,
   so one forecast is scored per fixture and it is the last one issued; UNRESOLVED — the
   database holds no match for the fixture yet (Cricsheet lag; an unrun import), shown
   with how many days past match_date it is; NO_RESULT — a match with no
   outcome_winner_opposition_id, counted and not scored; SCORED. State is computed on
   read from the store and the match tables — no column, no step, no scheduler — so an
   import moves a prediction to SCORED by itself. A fixture is resolved by exact
   match_date, both opposition ids in either order, format and gender; a match on a
   neighbouring date is not it, and say so in the code.
2. THE SCORES, reusing the harness's arithmetic rather than re-deriving it: the headline
   win probability against the outcome — Brier over every SCORED prediction with the
   base-rate Brier beside it (the base rate from the same rows), and the reliability
   curve in the harness's ten equal-width bins with n per bin; the served 10-90 totals —
   whether each actual innings total fell inside its served range, per innings, with n;
   and the elevens — how many of the predicted 22 appear in match_player for that match,
   shown per prediction and summarised, because a forecast for a side that did not play
   is a forecast of a different match; it is reported, never used to exclude. Where the
   arithmetic lives (go-app in Go, or ml-service behind an endpoint that takes the rows)
   is your call — say why in the PR; the harness's functions are the reference either
   way and a test pins one bin against them.
3. TWO POPULATIONS, NEVER POOLED. Put on the wire whether the served simulator for the
   format carried a shared factor (SimulateResponse gains it from the calibration; go-app
   carries it onto the prediction; the store keeps it as a column from this item on) and
   report coverage over the factored and factorless predictions separately with both
   denominators visible, the way summarize_folds does. A prediction stored before this
   field existed is in a third population, "unknown", shown as such.
4. THE SURFACE. A Track record tab: the summary numbers with their n, the reliability
   plot, the coverage per innings and per population, the state counts, and the list
   of predictions newest first with state, what was claimed, what happened, and the
   eleven overlap. Beside the served numbers, the harness's locked-window figures for
   the same run from xi_evaluate_report.json, labelled as the harness's, so a reader
   sees the record against the number the model was accepted on. No number is a
   verdict: nothing turns red at a threshold, and every figure carries its n. The
   ranges are the simulator's as served — no widening, no day/night adjustment (B-11 is
   open and the record will show it). Every labelled number is an L-1 key with its
   explainer reachable; add entries to ml/xi/glossary.py (the completeness gate names
   the missing ones).
5. VERIFY on the dev stack against the scratch database with a seeded history: store
   predictions for fixtures the scratch database already holds as played matches (a scenario, two
   forecasts of one fixture, one no-result, one unresolved) and show each lands in its
   state; then, on cricket_data, issue predictions through the Lab for real upcoming
   fixtures, run make import by hand when Cricsheet has them (or, if none has resolved
   by the time you finish, say so and show the UNRESOLVED count and days instead of
   inventing a result), and record what the tab showed. A miss on the record is the
   demonstration, not a problem.
6. TESTS AND DOCS. Go: each state from a fixture (table-driven), the superseding rule,
   the exact-date resolution, the scores against hand-computed values and one bin against
   the harness's function, the two populations kept apart, and a scratch-database
   integration test through the real handler; frontend: the tab per state, the n on
   every number, and the plot rendering an empty record without inventing a curve. Docs:
   README's tab list, docs/apis-backtest-and-ops.md (the endpoint and the states),
   docs/ml-and-training.md (the record beside the harness: what each is evidence of),
   make gen-architecture-map if a contract moved.

Acceptance (the P2-4 gate): the record on the dev stack shows every stored prediction in
exactly one named state; the scored ones carry Brier, reliability and coverage with their
n and the base rate beside them; an import that brings a predicted match moves it to
scored with no operator step; a miss is on the record; the factored and factorless
populations are never pooled; nothing on the surface is filtered by outcome; make
check-all green; coverage gates never move down; cricket_data's three counts unchanged.
Record in docs/PRODUCT_ROADMAP.md § 10 (the P2-4 row) and, if the record showed something
the harness did not, write it into docs/BUG_BACKLOG.md rather than into a claim. Then
stop and hand over the push and PR commands.
```

## 6. Phase 3 — one wedge, chosen by Phase 0 (not all three)

- **3a — Auction/draft module (valuation framing).** Pool distribution tracking during
  an auction; per player: projected output distribution for the buyer's likely XI and
  venue mix, replacement level by role among players still available, scarcity curve,
  value-vs-price flag; "on the fly" recommendations = re-ranking the remaining pool as
  slots fill. Built on L2-B + simulator (proven in T20). Never marketed as XI-picking.
- **3b — Underserved B2B.** White-label match reports, opposition analysis and squad
  reviews for women's teams, associate nations, emerging leagues; the harness's
  honesty is the sales deck. Small contracts, real references, no incumbent contest.
- **3c — Fantasy objective.** A points model (same infra, new target trained on the
  same events, same calibration discipline) powering projections with ranges — only if
  P0-2 clears it and P0-3 shows pull. Biggest market, biggest compliance weight.

## 7. Phase 4 — the moat, if Phases 1–3 earn it

The accumulating assets: the public track record; user-entered squad/availability data;
per-league calibration nobody else publishes; possibly licensed data deepening coverage.
Expansion candidates (evidence-gated, as ever): batting-order suggestion IF a future E3
re-run crosses its line on new data; T20 selection IF A-3-class work ever clears the
derived bar — the harness re-verdicts every retrain, so the product inherits upgrades
the moment they are real.

## 8. Risks, plainly

| risk | mitigation |
|---|---|
| No data moat; clone risk | Ship the trust asset and product speed; accumulate operational data (Phase 2) |
| Marketing drifts into claims the record contradicts | Every public claim maps to a harness number; the §8.8-class nulls are shown, not hidden |
| Fantasy/betting regulation | P0-2 before any 3c work; jurisdiction gating |
| Freshness cost eats the margin | **Decided (P0-4, § 2.1): recurring data cost zero, staleness paid instead** — "as of last import", the date visible, H-11 refusing past 14 days. The residual risk is the licence, not the price: Cricsheet grants none, and that must be settled with the project before anything ships or is sold |
| Solo-maintainer bus factor | The run pipeline, contracts and docs already assume operator-independence; keep it that way |
| The lab is a toy (week-3 retention fails) | Unmeasurable under the standing constraint: Phase 1's gate is functional acceptance (§ 3) and says so; the retention question waits with the deferred SaaS half until route (a) is revisited, and the pivot to 3a/3b waits with it |

## 9. Sequencing and effort

P0 (weeks, mostly not code — P0-1 is one harness PR) → P1 (the big build: months of
product engineering; the model layer is done) → P2 (ongoing ops, starts during P1) →
one wedge of P3 (months) → P4 (earned, not scheduled). Each phase gets item-by-item
kickoff prompts in the [EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) style when it
starts; Phase 1's are in § 3.2 (2026-09-06), Phase 2's in § 5.2 (2026-09-07).

Under the standing constraint the sequence reads differently. P0 is mostly not runnable
(§ 2: one item measured, one deferred, one skipped, one forced), and its exit gate closed
on route (a). P1 is a **locally-run prototype**, not a hosted product — the Team Lab
only: its months of product engineering exclude the SaaS plumbing, whose hosting, accounts and metering are a
running cost, and its "infra cost per active user" has no number to take until hosting is
in scope. P2 is four items on the existing stack, ingest stays manual and its availability
data is maintained lists (§ 5). P3's wedge, if one is chosen, is chosen by judgment. None of this shortens the model-layer work, which is done; it removes
the parts that cost money or need people, and says so.

## 10. Record of outcomes

| id | status |
|---|---|
| P0-1 | **measured, on free sources only** — the one licence-clean free series covers BBL/WBBL; 4.2 % of T20 joined, 0 % elsewhere; market ahead by 0.052 AUC with a 95 % interval spanning zero, so it does not resolve the market question (§ 2; [EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) § X-4) |
| P0-2 | ⏸ **deferred** (2026-09-04, standing constraint: needs paid counsel) — a prerequisite again before anything ships publicly or takes payment; until then the prototype has no verified disclaimer architecture and no jurisdictional clearance, an accepted gap (§ 2) |
| P0-3 | ✗ **skipped** (2026-09-04, standing constraint: interviews are human-subject research) — the Phase 3 wedge cannot be chosen on evidence; any later choice is a judgment call recorded as unevidenced (§ 2) |
| P0-4 | ✅ **answered as a decision** (2026-09-04; write-up in § 2.1). The system ships **"as of last import" with the as-of date visible**, and refuses a live prediction past H-11's limit rather than answering from stale ratings; paid feeds were out of scope, so there was no feed to quote and the answer was forced. **Recurring data cost: zero. What is paid instead: staleness** — measured on this box, served ratings through 2026-09-02, **2 days** against the 14-day limit, with the database's own latest match on the same date (nothing lost between import and serving), and the weekly cadence bounding the age at eight or nine days. **Recorded beside the cost line: the Cricsheet licence is unresolved** — no licence is stated for the match archive, only the author's criteria (free, derivatives allowed, corrections reported, not resold), which this prototype's use sits inside; it must be settled with the project directly before anything ships or is sold ([config-and-data.md](config-and-data.md) § Data-source licence register). **The decision is answered; the surfacing it implies is not** — § 2.1's audit names four gaps (the prediction payload carries no as-of date or run id; the Upcoming-match tab shows the date only when the prediction would be refused; the run manifest records `cutoff` but not `ratings_through`; go-app's `db_freshness` buckets and H-11 are two unreconciled rules, disagreeing today) and fixes none: the first two were Phase 1's P1-5 (§ 3.2, shipped), the last two are Phase 2's P2-1 and P2-2 (§ 5.2) |
| P0 exit gate | ✅ **closed — route (a), recorded 2026-09-06**: *"internal tool / prototype, no wedge chosen"*. Route (b) was declined. No Phase 3 wedge is chosen; no evidence for choosing one exists (P0-3 skipped); any later choice is recorded as an unevidenced judgment call (§ 2) |
| P1 | ✅ **closed — functional acceptance, recorded 2026-09-06.** All five items shipped (P1-1 #269, P1-5 #270, P1-2 #271, P1-3 #272, P1-4 #273), run in the order P1-1 → P1-5 → P1-2 → P1-3 → P1-4, and each of § 3's six functional-acceptance clauses is marked with the item that demonstrated it on the dev stack. **What was accepted, at its actual size: an internal prototype whose surfaces do what § 3 says they do**, checked against P1-4's inventory of every served and refused state. **What was not:** retention, pull and cost per active user were never measured — route (a) put all three out of scope (§ 2), along with the user study the gate as first written rested on — so this closes the build, not the market question, and P0-1's unresolved benchmark and P0-3's skipped interviews stand exactly where they stood. Two findings were recorded rather than folded in: B-8 (the selection's rating order can disagree with the display model) and B-10 (must-include reached the search as an empty lock), both in `docs/BUG_BACKLOG.md`; B-10 is fixed on the branch carrying this row. The SaaS half — accounts, saved scenarios, rate limits and metering, hosting, freemium tiers — is deferred with its reason recorded in § 3, and returns the day route (a) is revisited |
| P1-1 | ✅ **shipped** (2026-09-06) — one Lab surface, the toss toggle, the pools visible. The Upcoming-match tab **became** the Team Lab (`/lab`, `TeamLabTab` + `useTeamLab`) rather than gaining a sibling, so there is one surface on `POST /api/predict/team-selection`. New inputs: the **toss** (bat first / bowl first / unknown), and the constraints the endpoint always accepted but the UI never sent — minimum bowlers, the keeper, and must-include ids that join the pool whatever the window or the ledger says (an unreadable id stops the prediction rather than being dropped). **The toss reached the simulator for the first time**: `team1_bats_first` ran from `predictteam`'s simulation input through to `/simulate`, but `predictTeamRequest` had no field and the UI had no control, so nothing could set it; the field is nullable at every hop (absent = unknown = today's marginalised behaviour) and the response now carries `toss` — which batting order the numbers assume, and whether a named one could be used at all. Two §8.7 consequences: a named toss on a format with no innings length is reported *not honoured* with the reason instead of being ignored, and draws that disagree with the toss asked for (`toss_marginalised` against the request) are refused rather than served. One defect the toggle exposed and this fixes: `scorecard.innings1/innings2` were team1's and team2's innings whichever batted first, so "Innings 1 (India)" could sit beside "Australia bats first" — renamed `team1_innings`/`team2_innings` and labelled by side and batting position. **Verified on the dev stack** (run `20260906T083819Z-36689f80`, ratings through 2026-09-02, 4 days old): T20I India v Australia and ODI England v India come back `optimised true` with marginal values; T20 Mumbai Indians v Chennai Super Kings and TEST Australia v England come back `optimised false` with the H-17 / E5 note on screen and no marginal column; the three toss states give three different answers on the same fixture (unknown 73.1 % with innings 176/175 both orders averaged; team1 first 72.6 % with 189/170; team2 first 73.6 % with 168/183), each named on the surface; both pools render with window, size and the all-time pool one click away. `cricket_data` unchanged at 22,818 / 11,539,808 / 13,662. Frontend coverage ratcheted to 79/79/77/70 |
| P1-5 | ✅ **shipped** (2026-09-06, PR #270) — every served prediction carries its date and run id, and a refused one says why. **The payload:** `predictteam.Result` gains `ratings_through` and `run_id`, required, never omitted (§ 2.1's gap (1) closed). They come from a `served_ratings {run_id, ratings_through}` stamp ml-service now puts on every answer a prediction is assembled from — `/xi/optimize`, `/xi/predict-win`, `/simulate`, `/performance/predict` — read off the store that computed it (the same manifest and `state.last_date` `/xi/status` reports), rather than from one `/xi/status` read per prediction: a status read describes whatever is loaded at the moment of the read, and a reload can land between a prediction and that read. Nothing is cached past the request. go-app requires every stamp to agree; a prediction whose calls straddled a reload is `409 SERVED_RUN_CHANGED` naming both runs (§8.7), and an answer with no stamp is refused rather than read as an unknown date. **The surface:** the Lab shows "ratings as of *date* · run *id*" beside the headline probability, off its own payload (gap (2) closed); PredictionReadiness keeps the refused states, so the surface is dateless in no state. **The refusal, demonstrated end to end** on the dev stack with `XI_RATINGS_MAX_AGE_DAYS=1` as an env override on a branch ml-service (the served config untouched; the shared containers were not restarted): ml-service `/xi/status` read `fresh false, age_days 4, max_age_days 1, code RATINGS_STALE`; `POST /api/predict/team-selection` (T20I, India (men) v Australia (men), 2026-09-10) came back through go-app as **`503 {"code":"RATINGS_STALE","message":"ratings run through 2026-09-02 (4 days old, limit 1)","hint":"run the retrain step, then reload -- or raise XI_RATINGS_MAX_AGE_DAYS if this is deliberate"}`** — a 503, because go-app now relays an upstream 503 as itself instead of rewriting it to 502, which had made a named refusal read as a broken gateway. On the Lab the refusal rendered with the date, the age against the limit, ml-service's hint, the place in this UI that fixes it (Ops → Pipeline: Retrain, then Reload) and the code, and no number: the failed request clears the previous answer. With the limit back at 14 the same fixture answered **200** with `ratings_through 2026-09-02`, `run_id 20260906T083819Z-36689f80`, P(India) 73.1 % (P1-1's figure), and the Lab showed "ratings as of **2026-09-02** · run 20260906T083819Z-36689f80" beside it. `as_of` stays unreachable from the product. No glossary entry: the date and the run id are labels, not numbers L-1's gate covers; the chip carries its own one-sentence tooltip. **Tests:** Go — the adopt/refuse rule, the selection and both forecast paths refusing a mid-prediction run change, the client mapping the stamp, `relayStatus` passing 503, the handler answering 503 `RATINGS_STALE` and 409 `SERVED_RUN_CHANGED`, and two scratch-database integration tests through the real handler (a served payload carries both fields; a stale registry is a 503 with the code on the wire); ml-service — the stamp equals the status on both the optimised and rating-ordered paths and on every response model, a backtest is dated by its as-of state, a store with no manifest is refused, and the 503 reaches the route with the code, the date, the age and the hint; frontend — the date renders on success, the refusal renders on 503 with no number, and neither state is blank. `make check-all` green; no coverage gate could move — the gates measure ml-service 93.60 %, go-app 76.7 % and frontend 77.85/70.16/79.13/79.87, each rounding down to its existing threshold. `cricket_data` unchanged at 22,818 / 11,539,808 / 13,662 |
| P1-2 | ✅ **shipped** (2026-09-06) — Play mode, on one code path, with two of this document's claims corrected. **The path:** a hand-built eleven takes the *existing* predict path with the selection step replaced by the caller's answer — `team1_xi` / `team2_xi` on `POST /api/predict/team-selection` — because the numbers never came from `/xi/optimize` in the first place: the displayed probability is `/xi/predict-win`'s display model and the totals, ranges and scorecard are `/simulate`'s draws, both of which read the eleven they are given. `selection.objective` is `fixed`, `optimised` is false and no player carries a marginal value, because nothing was maximised; **Optimise again** returns to the searched eleven, which is the win-model search in T20I and ODI and the rating-ordered pick with its notice in T20 and TEST. Refused rather than repaired: an eleven that is not an eleven (`400 XI_INCOMPLETE` — a ten-man side would be a prediction for a match nobody plays), a player id that names nobody (`400 XI_PLAYER_UNKNOWN`), one side pinned and the other searched, and the same player twice. **Constraints are checked, never applied:** ml-service's `/xi/predict-win` optionally checks the eleven it is scoring against the constraints it was sent with and reports the counts — the bowler count from `contract.is_bowling_option` and the keeper flag from the served vectors, so "five bowlers" means in the chip what it means inside the search — and go-app assembles them into a `constraints` block naming the missing must-include players. A pinned prediction whose answer carries no check is refused rather than shown as met. **Verified on the dev stack** (run `20260906T083819Z-36689f80`, ratings through 2026-09-02): on T20I India v Australia the searched eleven reads 73.1 %, one swap re-scores to 71.2 % with the delta shown as **−1.9 pp** beside **+2.2 runs (+1.0 / +2.0)** on India's innings; removing a player leaves "this eleven is not scored yet" over the previous answer rather than scoring ten; adding one back with `min_bowlers` 8 re-scores to 69.8 % and shows **Bowlers 5 of 8 — broken** and **Bowlers 7 of 8 — broken** with "scored as you built it; nothing was substituted"; Optimise again returns a searched eleven under the new constraint. **Clause 3, demonstrated:** `scripts/probes/p1_2_play_mode.py` through the real stack — pinning the searched eleven reproduces Optimise's probability with a gap of **0.0000000000** in all four formats (one code path, asserted rather than argued), and **0 falls in 20 dominating upgrades** (T20I 2, ODI 2, T20 8, TEST 8), where an upgrade is a swap for a player at least as good on every one of `contract.PLAYER_VECTOR_KEYS`. **Clause 2, corrected:** 30 timed re-scores per format, client to client, at the served 2,000 draws — T20I median **370.5 ms** / p95 738.6, ODI **388.0** / 532.8, T20 **399.0** / 516.4, TEST **354.9** / 471.0. Where it goes, timed in-process on one T20I `/simulate`: the performance model's per-player forecasts **295.6 ms**, `simulate_match` at 2,000 draws **9.4 ms**, summarising the draws 5.5 ms, the display probability 6.8 ms, assembling the rows 0.8 ms; `/xi/predict-win` is 6–8 ms of the total and go-app's own work (two pool queries, the fixture resolution, two hops) about 40 ms. **So the ~10 ms this document claimed was the draw loop alone** — the harness's `ms_per_fixture_at_default_samples`, 9.5–9.8 there — for a fixture whose forecasts were already computed, and § 1 and § 3 now say what a user waits for instead. Measured with the branch running as host processes against the shared Postgres and the shared containers' own run; the same `/simulate` payload against the containerised ml-service is 145.4 ms median against the host process's 317.2, so inside the dev stack's containers the re-score would land near 200 ms — the claim is out by a factor of 20–40 either way, which is why no draw count was lowered and no cache added to chase it. **One finding recorded rather than fixed:** the same probe's diagnostic arm swaps for a player the *rating order* ranks higher without dominating him, and the displayed probability falls in 0/8 T20I, 1/8 ODI, 4/8 T20 and 6/8 TEST cases (worst −0.0805) — `docs/BUG_BACKLOG.md` § B-8. **Tests:** Go — the pinned selection path, every refusal, the constraint report, the client sending constraints only for a pinned eleven, and three scratch-database integration tests through the real handler; ml-service — the check reports the counts, a broken constraint comes back broken and unrepaired, and checking moves no probability; frontend — the delta arithmetic as pure functions, the Play-mode state machine (an edit that leaves ten players does not re-score; the delta is measured against the answer the change was made from), and the board, chips and delta rendering. `make check-all` green; **frontend coverage ratcheted to 80/80/78/71**, go-app 76.8 % and ml-service 93.62 % each rounding down to the existing threshold. `cricket_data` unchanged at 22,818 / 11,539,808 / 13,662 |
| P1-3 | ✅ **shipped** (2026-09-06) — the "why this player" card, on every selected player, showing what the selection consumed and nothing else. **The rule was applied, not asserted:** § 4 now carries the field-by-field map from each number on the card to the code that produced it, and the four fields it first sketched that are **omitted with their reasons** — a *trajectory* (the served state holds decayed accumulators as of one date, not a history, so the objective consumed no earlier value; form appears only as the decayed rates inside the selection rating and its standing), a *must-include* role (go-app puts a required id into the pool and sends ml-service an empty `must_include`, so the search never treats anyone as required and may leave him out — a "required" chip would name a constraint nothing applied), a *top-order anchor* role (`exp_bat_position` is read by the performance model, never by the objective), and an *interval on the beats-whom gap* (one objective evaluation per candidate swap yields a point estimate; the card says so instead of inventing one). The same list is the card component's doc-comment, so the rule travels with the code. **New on the wire:** `/xi/optimize` gains `selection_reasons` per selected player — `roles` (from `_Pool.is_keeper` / `is_bowler`, the same predicates the constraints and the `has_keeper` / `n_bowlers` features read), `selection_rating` (`rating_order_score`, extracted so `_greedy_seed` and the card cannot compute different composites), `rating_percentile` and `pool_size` (standing within the pool *as served*), and `best_alternative` (the best excluded pool player over exactly `_best_neighbour`'s single-swap neighbourhood, with the P(win) that swap costs) — and go-app carries it onto each `SelectedPlayer` as `selection_reason`, resolving the alternative's registry id to a player id and name. An alternative the pool cannot resolve is reported on the wire with its reason, never dropped (§8.7). `selection_roles` joins the H-24 contract, asserted from all three components. **Verified on the dev stack** (branch ml-service and go-api as host processes against the shared Postgres and run `20260906T083819Z-36689f80`, ratings through 2026-09-02): T20I India v Australia comes back `optimised true`, P(India) 0.7312, with a reason on all 22 players — pools of 26 and 24, percentiles 12 to 100, and every beats-whom gap positive (0.0011 to 0.1403), which is what a converged single-swap search should give; TEST Australia v England comes back `optimised false` with a reason on all 22, **no marginal value and no alternative anywhere**, percentiles 40.9 to 100 over pools of 18 and 23. **B-8's honest half:** the rating-ordered card says the ordering it shows is the selection's own composite and not the win model's ranking, and that swapping in a higher-rated player can move the displayed probability down — the sentence a user needs before acting on the order; P1-4 carries the other half. **Cost, measured rather than assumed:** the best-alternative sweep is **11.4 ms median** in-process on a 26-player T20I pool, beside the search's 383.8 ms and the marginal values' 1.7 ms, so about 46 ms on an Optimise (four win-objective calls); **Play mode's re-score is untouched**, because it makes no `/xi/optimize` call at all, and P1-2's 355–399 ms stands. **Glossary:** four new L-1 entries — `xi_role`, `selection_rating`, `rating_percentile`, `next_best_gap` — reachable from every labelled number on the card. **Tests:** ml-service — the composite is the order the rating-ordered pick uses, percentiles run 0 to 100 over the pool, roles agree with `is_bowling_option` and the keeper flag, the rating-ordered path scores no alternative, the best alternative equals a swap re-scored through the objective, no gap is negative on a converged search, an unpooled player gets no entry, and both `/xi/optimize` paths carry the block; go-app — the alternative resolves to a player id and name, an unresolvable one says so on the wire, the reasons come from the same round as the marginal values, Play mode carries none, the client maps the block on both objectives, and a scratch-database integration test through the real handler shows a rating-ordered payload carrying a reason per player with no alternative; frontend — the two card states, the rating-ordered one asserted to carry no marginal value and no beats-whom line even when handed one, the point-estimate sentence, the negative-marginal reading, and a control on every row. `make check-all` green; coverage gates never moved down — the frontend's statements gate ratcheted 78 -> 79, and **go-app's did not move, for a reason worth recording**: it measured 77.0 % locally and 76.9 % in CI, and the difference is exactly one statement in an unrelated package. `ctxReader.Read`'s cancellation branch is covered on a fast machine and not on the CI runner, because the test asserting it only asserts that `Fetch` errors, which it does whether or not the cancellation reaches that reader. The total sits on the 76.95 boundary, so that one statement decides the rounded figure. Diagnosed by diffing the CI run's own `coverage.out` artifact against a local profile — they differ in that one function and nothing else — and recorded as `docs/BUG_BACKLOG.md` § B-9; the threshold stays at main's 76, which is 76.9 rounded down. `cricket_data` unchanged at 22,818 / 11,539,808 / 13,662 |
| P1-4 | ✅ **shipped** (2026-09-06) — the sweep: every honesty surface, on every state of the Lab, checked against an inventory rather than a feeling. **The inventory** (in the PR, one row per number: what stands beside it, its range, its explainer key, its date) walked eight served states — T20I and ODI optimised, T20 and TEST rating-ordered, the three toss states, a must-include request, Play mode after a swap — and four refused ones. **What it changed on the surface:** (1) *ranges* — every point with a 10–90 range on the wire shows it in every state; the one point the stack serves without a range (wickets on the performance-quantiles path, an expectation with P(0/1/2+) and no quantiles) reads "(no range)" and no interval is derived from the pmf; the ranges are the simulator's as served, X-2's day/night gap recorded in the `innings_total` explainer's band rather than adjusted for; (2) *the not-optimised notice* — an `Alert` at the XI from `selection.note`, titled for the state it is in ("picked by rating" / "your eleven"), and a response with no note says so instead of inventing a reason; the win probability beside it is labelled "the display model's read of this rating-ordered eleven / the eleven you built, not the result of a search"; (3) *the date* — one `RatingsAsOf` component renders "ratings as of *date* · run *id*" on every served prediction and in the stale readiness notice; the nothing-loaded notice says there is no date to show; (4) *the explainers* — every labelled number opens an L-1 entry, including the delta's labels in Play mode, and the win probability's source and the forecast's source are shown **by name** with an entry each: five new glossary keys, `innings_total`, `win_probability_source_display`, `win_probability_source_simulator`, `forecast_source_simulator`, `forecast_source_performance_quantiles`, keyed `<field>_<value>` off the two source vocabularies that now sit in the H-24 contract (`win_probability_sources`, `forecast_sources`; two sides, go-app and the frontend, because go-app decides both); `forecast` reaches the frontend for the first time — `predictteam.Result` always carried it and the Lab never read it, so the TEST card's "no innings length" sentence was the surface's own and is now the wire's. **B-8's other half, on the surface only:** a searched board is ordered by the marginal value the response already carries and says so ("the objective's own ranking of this eleven, which is what a swap here is measured against"); the rating-ordered board says it is in rating order and not the win model's ranking; the hand-built board says nothing ranks it; and Play mode carries the sentence about which ordering the swap guarantee holds under — dominance on every rated axis, not the rating order the T20/TEST eleven is listed in. No model and no selection policy changed. **The must-include label is now true, and the check it names now exists on the Optimise path:** the input reads "added to the pool, checked after"; go-app's `selection.must_include` names, per side, how many were asked for and each one the selection left out (an id the pool cannot resolve still stops the prediction), so "0 of 1 in the eleven · Ashok Sharma left out" is on screen where before nothing was; the real fix — a lock the optimiser honours — is `docs/BUG_BACKLOG.md` § B-10, not this item. **One wire change beyond the surface:** a transport failure to ml-service was `500 INTERNAL` with a dial string for a message; it is now `502 ML_UNREACHABLE` with the endpoint, the reason and a hint (§8.7 — the dependency that is down is named, not this service blamed). **Two states the gate did not anticipate, added to § 3's clauses 4 and 6:** Play mode's `fixed` selection is a third selection state and the Lab was titling it "Rating-ordered 11"; and the range-less wickets point above. **Demonstrated on the dev stack** with branch go-api and ml-service host processes beside the shared containers (which were not restarted — and the shared ml-service image, built 16:52Z, predates P1-3's 17:55Z merge, so it serves no `selection_reasons`; the success states went through a branch ml-service on the shared run instead): every served state through `POST /api/predict/team-selection` on run `20260906T083819Z-36689f80`, ratings through 2026-09-02 — T20I India v Australia 73.1 % display / 53.5 % simulated, innings 176 (142–220) and 175 (141–215); ODI 63.9 %; T20 MI v CSK rating-ordered 58.0 % with the E5 note; TEST Australia v England rating-ordered 51.3 % with the H-17 note, `forecast.source performance_quantiles` and its note, no scorecard, 22 wickets points with no range; the three toss states 73.1 / 72.6 / 73.6 % naming their order; a TEST toss reported not honoured; must-include `[11877, 395]` reporting Ashok Sharma left out and Bumrah in; Play mode swapping AR Patel (marginal −2.9 pp) for Ashok Sharma re-scoring 70.0 % (−3.1 pp) with both constraint blocks met — and each of these on the branch frontend on `localhost:5174` against that go-api, with the explainer popover opened from "source: display model". **The four refusals, each on the wire and on screen:** `503 RATINGS_STALE` ("ratings run through 2026-09-02 (4 days old, limit 1)") from a branch ml-service with `XI_RATINGS_MAX_AGE_DAYS=1`, and the readiness notice before a request showing the same date through the same component; `503 XI_MODEL_UNAVAILABLE` from a branch ml-service with an empty `MODELS_DIR`, and the readiness notice saying no run is loaded and no date can be shown; `502 ML_UNREACHABLE` from a go-api pointed at a closed port; `400 FIXTURE_CROSS_GENDER` on the wire only — the picker cannot build one, which is the point. In every refusal the Lab shows the message, the hint, the remedy and the code, and neither a number nor a date. **Tests:** frontend — an "honesty surfaces" suite per state (optimised, rating-ordered, rating-ordered with no reason served, Play mode after a swap, the three toss states, the must-include outcomes, stale before a request, nothing loaded, cross-gender, unreachable) asserting the notice, the ranges, the date and the explainer links; `boardOrder` and `RatingsAsOf` unit tests; the source vocabularies asserted against the contract; go-app — the must-include report in both outcomes and its absence in Play mode, the unreachable client error, the source vocabularies; ml-service — the five entries pass the completeness gate. `make check-all` green; **frontend lines gate ratcheted 80 → 81** (measured 81.29 / 80.92 / 79.39 / 71.61); go-app measured 77.0 % locally and stays at 76 (B-9: CI reads 76.9); ml-service 93.64 % rounds to its 93. `cricket_data` unchanged at 22,818 / 11,539,808 / 13,662 |
| P2 | **open — 2026-09-07, written for route (a)** (§ 5): four items on the existing stack — one freshness verdict (P2-1, the priority), the manifest's `ratings_through` (P2-2), the prediction store (P2-3) and the internal track record (P2-4) — with a functional-acceptance exit gate. Dropped with the reason recorded: the public track-record page (no public, no moat), scheduled ingest (the user runs `make cadence` by hand; H-11 will refuse until it is run, visibly), a licensed availability feed (standing constraint), and a further availability item (judged covered by D-12's pool, picker and ledger for one user) |
| P2-1 | ✅ **shipped** (2026-09-07, PR #279) — one freshness verdict, assembled once, read everywhere. **What shipped:** `/ops/status` carries a `freshness` object with three named facts and nothing else — `served` (H-11's verdict copied through from ml-service: `fresh`, `age_days`, `max_age_days`, `ratings_through`, `code`, plus the word that spells it — `fresh` | `stale` | `not_loaded` | `unknown` — and it is the only badge and the only thing that says a prediction would be refused), `database` (per format, the latest imported match date, its age in days and the match count, as facts with no bucket and no status of their own, plus a `note` when there is no date), and `retrain_due` (`up_to_date` | `retrain_due` | `unknown`, with `days_behind`, the date and the format that holds it — the B-2 state a green pipeline once hid). **`db_freshness` and its `overall` are deleted, not kept beside it**; `db_completeness` asks a different question ("is the import empty?") and is untouched, and `readStatus`'s private `ok | stale | missing | unknown` went with the buckets it read. **One threshold in the whole system:** `ml.ratings_max_age_days`, applied by ml-service where the verdict is computed; go-app holds no copy and reads the limit off the verdict. The vocabulary is declared once in `internal/freshness`, published in `contracts/ops-console.contract.json` (`freshness_statuses`, `retrain_statuses`, `ratings_stale_code`) and asserted from all three components (H-24). **Every surface reads the one object:** the Health tab's Ratings line (which now reads `/ops/status` rather than its own health poll), the Ops console's new Freshness card and the runs panel's badge, the Workbench's loaded-run card, the system map's `ratings_through` / `ratings_age` bindings (re-pointed off `xi_status` and `artifacts.ratings`, with `ratings_freshness` and `retrain_due` added), and the Lab's readiness notice — with a go-app test that the verdict on `/ops/status` and the verdict on `/api/ml/xi-status` describe the same store field for field. **Observation 1 — the disagreement, reproduced and then gone** (dev stack, 2026-09-07, served run `20260906T083819Z-36689f80`, ratings through **2026-09-02**): the old `/ops/status` from the running main build read `db_freshness.overall.status` **stale** — TEST's latest match `2026-08-27`, **11 days**, past the 7-day bucket — while `artifacts.ratings` read **fresh**, 5 days of 14, on the same payload; the branch build of the same box reads one verdict, `freshness.served.status` **fresh** (5 of 14, through 2026-09-02), with TEST's lag beside it as `latest_match_date 2026-08-27, age_days 11, match_count 3115` and no status of its own, and `retrain_due up_to_date` (the import's latest match is T20 2026-09-02, 0 days behind). **Observation 2 — lowering the limit refuses everywhere, naming one date** (a branch ml-service with `XI_RATINGS_MAX_AGE_DAYS=3`, the served config untouched, on the same run): `/ops/status` reads `served {status stale, fresh false, age_days 5, max_age_days 3, ratings_through 2026-09-02, code RATINGS_STALE}`, `/api/ml/xi-status` reads the identical verdict, and `POST /api/predict/team-selection` (T20I Australia v England) answers **503 `RATINGS_STALE`, "ratings run through 2026-09-02 (5 days old, limit 3)"** with the hint naming retrain then reload — the badge, the readiness notice's input and the refusal all naming **2026-09-02**. **Tests:** go-app — the three facts from a fake probe and an httptest ml-service, a stale verdict copied rather than recomputed (5 days against a limit of 3, which the deleted bucket would have called `ok`), retrain-due true and false, nothing-loaded, an unreachable ml-service reading `unknown`, the database's silences named on the wire (§8.7), the contract vocabulary, the two-surface parity test and one asserting `db_freshness` is gone and `freshness` has exactly three keys; frontend — the Freshness card's three facts, the Ops badge and the readiness notice agreeing on one payload and naming the same date when it refuses, the Health tab and Workbench cards reading the object, and the vocabulary asserted against the contract; ml-service — the refusal's code and the status verdict's code both asserted against the contract, and nothing-loaded carrying no code and no invented date. `make check-all` green; coverage gates never moved down — **frontend functions 80 → 81 and branches 71 → 73**, each verified against the CI run's own figure rather than a local one (B-9): run 34088473978 measured 81.89 lines / 81.45 functions / 79.99 statements / 73.07 branches, the same four numbers as the local run, and the raised gates pass on it; go-app measured **77.2 %** in CI (run 34088473931) as it did locally and stays at 77, its floor; ml-service 93.86 % rounds to its 93. `cricket_data` unchanged at 22,818 / 11,539,808 / 13,662 |
| P2-2 | open — not started. The run manifest records `ratings_through` beside `cutoff`, asserted against the state at load (§ 2.1 gap (3); prompt in § 5.2) |
| P2-3 | open — not started. The prediction store: every issued answer persisted verbatim with its run and date (prompt in § 5.2) |
| P2-4 | open — not started. The internal track record: stored predictions scored on read, misses included, factorless simulator predictions never pooled (prompt in § 5.2) |
| P3 | open — **no wedge chosen** (route (a), § 2): P0-3 was skipped, so no evidence for a choice exists; if one is ever made it is recorded as an unevidenced judgment call, never as validated |
