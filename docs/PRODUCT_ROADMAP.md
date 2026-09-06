# Product roadmap: from an honest prediction system to something people pay for

**Status: Phase 0 closed on route (a), Phase 1 open as the Team Lab (2026-09-06).**
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
| P0-4 | **Freshness decision** — ✅ **answered as a decision** (2026-09-04; the write-up is § 2.1 below). With paid feeds out of scope there was no licensed feed to quote against, so the comparison collapsed and the answer was forced: the system ships **"as of last import", with the as-of date visible**, and refuses a live prediction against ratings past H-11's limit rather than answering from a squad that has moved on. The write-up records the decision, audits the freshness machinery that actually exists (and names what does not), measures the staleness this box lives with, and puts the **Cricsheet licence question beside the cost line**, where it belongs. X-2 later put a second thing there: the weather source is free **non-commercially only**, so the zero is conditional on the use staying non-commercial (§ 2.1) | **The recurring data cost is zero, and what is paid instead is staleness.** Measured 2026-09-04 on this box: served ratings run through 2026-09-02, 2 days against the 14-day limit, and the weekly cadence bounds that at eight or nine days in normal operation. The decision is made; the *surfacing* it implies is split: the as-of date on the prediction itself, not only on the operator tabs, is **Phase 1 work (§ 3.2, P1-5)** — a Lab whose pitch is honest uncertainty cannot serve a dateless number — while the manifest and the two-rules reconciliation stay Phase 2 (§ 5); § 2.1 names all four gaps without closing them |

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
nothing reconciles them or says which one a badge would mean. None belong to this item,
which is a decision and an audit. Gaps (1) and (2) — the dateless payload and the dateless
successful prediction — were **Phase 1's P1-5** (§ 3.2, re-homed 2026-09-06, shipped in
PR #270): the Lab cannot honestly show a number without its date. Gaps (3) and (4) stay
Phase 2's freshness badge (§ 5).

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

1. **Correctness against the serving stack.** The Lab answers every call correctly against
   the existing stack: the XI, the win probability, the totals and the scorecard it shows
   are the ones `POST /api/predict/team-selection` returns for the same inputs, with no
   client-side arithmetic on top of them.
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
   interval from the pmf; (c) *a must-include id on Optimise* joins the pool and is not
   enforced, so the answer now names any the selection left out (`selection.must_include`)
   and the input says "added to the pool, checked after". A served answer with no
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
2. **A "must-include" role.** There is no such optimiser constraint on the predict path:
   go-app puts a required id into the *pool* (P1-1) and sends ml-service an empty
   `must_include`, so the search never treats anyone as required and may leave a
   must-include player out. A chip saying "required" would name a constraint nothing
   applied. (Play mode's constraint chips do report must-include, because there the check
   really is made — P1-2.) The role vocabulary is therefore two values, `keeper` and
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

## 5. Phase 2 — data operations as a product

Scheduled ingest (A-5's cadence, in production), squad/availability data as **maintained
lists, not a licensed feed** — a feed is a paid source and the standing constraint rules it
out, so availability is what operators and users record, and D-12's retirement ledger
(a user's flag, promoted to a fact only when held data corroborates it) is the first piece
of that list; the pool-accuracy problem is a product problem now — the freshness badge
everywhere (P0-4's decision, on every surface, closing the two gaps § 2.1 leaves once
Phase 1's P1-5 has dated the payload and the prediction: the manifest's missing
`ratings_through`, and `db_freshness` reconciled with H-11), and the **public track
record page**: every published prediction scored after the match, cumulative calibration
plotted, misses included. This page is
the moat seed: it compounds, and a competitor who won't publish theirs loses the
comparison by refusing it.

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
starts; Phase 1's are in § 3.2 (2026-09-06).

Under the standing constraint the sequence reads differently. P0 is mostly not runnable
(§ 2: one item measured, one deferred, one skipped, one forced), and its exit gate closed
on route (a). P1 is a **locally-run prototype**, not a hosted product — the Team Lab
only: its months of product engineering exclude the SaaS plumbing, whose hosting, accounts and metering are a
running cost, and its "infra cost per active user" has no number to take until hosting is
in scope. P2's availability data is maintained lists. P3's wedge, if one is chosen, is
chosen by judgment. None of this shortens the model-layer work, which is done; it removes
the parts that cost money or need people, and says so.

## 10. Record of outcomes

| id | status |
|---|---|
| P0-1 | **measured, on free sources only** — the one licence-clean free series covers BBL/WBBL; 4.2 % of T20 joined, 0 % elsewhere; market ahead by 0.052 AUC with a 95 % interval spanning zero, so it does not resolve the market question (§ 2; [EXTERNAL_DATA_PLAN.md](EXTERNAL_DATA_PLAN.md) § X-4) |
| P0-2 | ⏸ **deferred** (2026-09-04, standing constraint: needs paid counsel) — a prerequisite again before anything ships publicly or takes payment; until then the prototype has no verified disclaimer architecture and no jurisdictional clearance, an accepted gap (§ 2) |
| P0-3 | ✗ **skipped** (2026-09-04, standing constraint: interviews are human-subject research) — the Phase 3 wedge cannot be chosen on evidence; any later choice is a judgment call recorded as unevidenced (§ 2) |
| P0-4 | ✅ **answered as a decision** (2026-09-04; write-up in § 2.1). The system ships **"as of last import" with the as-of date visible**, and refuses a live prediction past H-11's limit rather than answering from stale ratings; paid feeds were out of scope, so there was no feed to quote and the answer was forced. **Recurring data cost: zero. What is paid instead: staleness** — measured on this box, served ratings through 2026-09-02, **2 days** against the 14-day limit, with the database's own latest match on the same date (nothing lost between import and serving), and the weekly cadence bounding the age at eight or nine days. **Recorded beside the cost line: the Cricsheet licence is unresolved** — no licence is stated for the match archive, only the author's criteria (free, derivatives allowed, corrections reported, not resold), which this prototype's use sits inside; it must be settled with the project directly before anything ships or is sold ([config-and-data.md](config-and-data.md) § Data-source licence register). **The decision is answered; the surfacing it implies is not** — § 2.1's audit names four gaps (the prediction payload carries no as-of date or run id; the Upcoming-match tab shows the date only when the prediction would be refused; the run manifest records `cutoff` but not `ratings_through`; go-app's `db_freshness` buckets and H-11 are two unreconciled rules, disagreeing today) and fixes none: the first two are Phase 1's P1-5 (§ 3.2), the last two Phase 2's freshness badge (§ 5) |
| P0 exit gate | ✅ **closed — route (a), recorded 2026-09-06**: *"internal tool / prototype, no wedge chosen"*. Route (b) was declined. No Phase 3 wedge is chosen; no evidence for choosing one exists (P0-3 skipped); any later choice is recorded as an unevidenced judgment call (§ 2) |
| P1 | **all five items shipped (2026-09-06) — the Team Lab only.** Five items with kickoff prompts in § 3.2, run in the order P1-1 → P1-5 → P1-2 → P1-3 → P1-4, gated on functional acceptance (§ 3's six clauses, each now marked with the item that demonstrated it; closing the phase is the user's call, not this row's). The SaaS half — accounts, saved scenarios, rate limits and metering, hosting, freemium tiers — is deferred with its reason recorded in § 3, and returns the day route (a) is revisited |
| P1-1 | ✅ **shipped** (2026-09-06) — one Lab surface, the toss toggle, the pools visible. The Upcoming-match tab **became** the Team Lab (`/lab`, `TeamLabTab` + `useTeamLab`) rather than gaining a sibling, so there is one surface on `POST /api/predict/team-selection`. New inputs: the **toss** (bat first / bowl first / unknown), and the constraints the endpoint always accepted but the UI never sent — minimum bowlers, the keeper, and must-include ids that join the pool whatever the window or the ledger says (an unreadable id stops the prediction rather than being dropped). **The toss reached the simulator for the first time**: `team1_bats_first` ran from `predictteam`'s simulation input through to `/simulate`, but `predictTeamRequest` had no field and the UI had no control, so nothing could set it; the field is nullable at every hop (absent = unknown = today's marginalised behaviour) and the response now carries `toss` — which batting order the numbers assume, and whether a named one could be used at all. Two §8.7 consequences: a named toss on a format with no innings length is reported *not honoured* with the reason instead of being ignored, and draws that disagree with the toss asked for (`toss_marginalised` against the request) are refused rather than served. One defect the toggle exposed and this fixes: `scorecard.innings1/innings2` were team1's and team2's innings whichever batted first, so "Innings 1 (India)" could sit beside "Australia bats first" — renamed `team1_innings`/`team2_innings` and labelled by side and batting position. **Verified on the dev stack** (run `20260906T083819Z-36689f80`, ratings through 2026-09-02, 4 days old): T20I India v Australia and ODI England v India come back `optimised true` with marginal values; T20 Mumbai Indians v Chennai Super Kings and TEST Australia v England come back `optimised false` with the H-17 / E5 note on screen and no marginal column; the three toss states give three different answers on the same fixture (unknown 73.1 % with innings 176/175 both orders averaged; team1 first 72.6 % with 189/170; team2 first 73.6 % with 168/183), each named on the surface; both pools render with window, size and the all-time pool one click away. `cricket_data` unchanged at 22,818 / 11,539,808 / 13,662. Frontend coverage ratcheted to 79/79/77/70 |
| P1-5 | ✅ **shipped** (2026-09-06, PR #270) — every served prediction carries its date and run id, and a refused one says why. **The payload:** `predictteam.Result` gains `ratings_through` and `run_id`, required, never omitted (§ 2.1's gap (1) closed). They come from a `served_ratings {run_id, ratings_through}` stamp ml-service now puts on every answer a prediction is assembled from — `/xi/optimize`, `/xi/predict-win`, `/simulate`, `/performance/predict` — read off the store that computed it (the same manifest and `state.last_date` `/xi/status` reports), rather than from one `/xi/status` read per prediction: a status read describes whatever is loaded at the moment of the read, and a reload can land between a prediction and that read. Nothing is cached past the request. go-app requires every stamp to agree; a prediction whose calls straddled a reload is `409 SERVED_RUN_CHANGED` naming both runs (§8.7), and an answer with no stamp is refused rather than read as an unknown date. **The surface:** the Lab shows "ratings as of *date* · run *id*" beside the headline probability, off its own payload (gap (2) closed); PredictionReadiness keeps the refused states, so the surface is dateless in no state. **The refusal, demonstrated end to end** on the dev stack with `XI_RATINGS_MAX_AGE_DAYS=1` as an env override on a branch ml-service (the served config untouched; the shared containers were not restarted): ml-service `/xi/status` read `fresh false, age_days 4, max_age_days 1, code RATINGS_STALE`; `POST /api/predict/team-selection` (T20I, India (men) v Australia (men), 2026-09-10) came back through go-app as **`503 {"code":"RATINGS_STALE","message":"ratings run through 2026-09-02 (4 days old, limit 1)","hint":"run the retrain step, then reload -- or raise XI_RATINGS_MAX_AGE_DAYS if this is deliberate"}`** — a 503, because go-app now relays an upstream 503 as itself instead of rewriting it to 502, which had made a named refusal read as a broken gateway. On the Lab the refusal rendered with the date, the age against the limit, ml-service's hint, the place in this UI that fixes it (Ops → Pipeline: Retrain, then Reload) and the code, and no number: the failed request clears the previous answer. With the limit back at 14 the same fixture answered **200** with `ratings_through 2026-09-02`, `run_id 20260906T083819Z-36689f80`, P(India) 73.1 % (P1-1's figure), and the Lab showed "ratings as of **2026-09-02** · run 20260906T083819Z-36689f80" beside it. `as_of` stays unreachable from the product. No glossary entry: the date and the run id are labels, not numbers L-1's gate covers; the chip carries its own one-sentence tooltip. **Tests:** Go — the adopt/refuse rule, the selection and both forecast paths refusing a mid-prediction run change, the client mapping the stamp, `relayStatus` passing 503, the handler answering 503 `RATINGS_STALE` and 409 `SERVED_RUN_CHANGED`, and two scratch-database integration tests through the real handler (a served payload carries both fields; a stale registry is a 503 with the code on the wire); ml-service — the stamp equals the status on both the optimised and rating-ordered paths and on every response model, a backtest is dated by its as-of state, a store with no manifest is refused, and the 503 reaches the route with the code, the date, the age and the hint; frontend — the date renders on success, the refusal renders on 503 with no number, and neither state is blank. `make check-all` green; no coverage gate could move — the gates measure ml-service 93.60 %, go-app 76.7 % and frontend 77.85/70.16/79.13/79.87, each rounding down to its existing threshold. `cricket_data` unchanged at 22,818 / 11,539,808 / 13,662 |
| P1-2 | ✅ **shipped** (2026-09-06) — Play mode, on one code path, with two of this document's claims corrected. **The path:** a hand-built eleven takes the *existing* predict path with the selection step replaced by the caller's answer — `team1_xi` / `team2_xi` on `POST /api/predict/team-selection` — because the numbers never came from `/xi/optimize` in the first place: the displayed probability is `/xi/predict-win`'s display model and the totals, ranges and scorecard are `/simulate`'s draws, both of which read the eleven they are given. `selection.objective` is `fixed`, `optimised` is false and no player carries a marginal value, because nothing was maximised; **Optimise again** returns to the searched eleven, which is the win-model search in T20I and ODI and the rating-ordered pick with its notice in T20 and TEST. Refused rather than repaired: an eleven that is not an eleven (`400 XI_INCOMPLETE` — a ten-man side would be a prediction for a match nobody plays), a player id that names nobody (`400 XI_PLAYER_UNKNOWN`), one side pinned and the other searched, and the same player twice. **Constraints are checked, never applied:** ml-service's `/xi/predict-win` optionally checks the eleven it is scoring against the constraints it was sent with and reports the counts — the bowler count from `contract.is_bowling_option` and the keeper flag from the served vectors, so "five bowlers" means in the chip what it means inside the search — and go-app assembles them into a `constraints` block naming the missing must-include players. A pinned prediction whose answer carries no check is refused rather than shown as met. **Verified on the dev stack** (run `20260906T083819Z-36689f80`, ratings through 2026-09-02): on T20I India v Australia the searched eleven reads 73.1 %, one swap re-scores to 71.2 % with the delta shown as **−1.9 pp** beside **+2.2 runs (+1.0 / +2.0)** on India's innings; removing a player leaves "this eleven is not scored yet" over the previous answer rather than scoring ten; adding one back with `min_bowlers` 8 re-scores to 69.8 % and shows **Bowlers 5 of 8 — broken** and **Bowlers 7 of 8 — broken** with "scored as you built it; nothing was substituted"; Optimise again returns a searched eleven under the new constraint. **Clause 3, demonstrated:** `scripts/probes/p1_2_play_mode.py` through the real stack — pinning the searched eleven reproduces Optimise's probability with a gap of **0.0000000000** in all four formats (one code path, asserted rather than argued), and **0 falls in 20 dominating upgrades** (T20I 2, ODI 2, T20 8, TEST 8), where an upgrade is a swap for a player at least as good on every one of `contract.PLAYER_VECTOR_KEYS`. **Clause 2, corrected:** 30 timed re-scores per format, client to client, at the served 2,000 draws — T20I median **370.5 ms** / p95 738.6, ODI **388.0** / 532.8, T20 **399.0** / 516.4, TEST **354.9** / 471.0. Where it goes, timed in-process on one T20I `/simulate`: the performance model's per-player forecasts **295.6 ms**, `simulate_match` at 2,000 draws **9.4 ms**, summarising the draws 5.5 ms, the display probability 6.8 ms, assembling the rows 0.8 ms; `/xi/predict-win` is 6–8 ms of the total and go-app's own work (two pool queries, the fixture resolution, two hops) about 40 ms. **So the ~10 ms this document claimed was the draw loop alone** — the harness's `ms_per_fixture_at_default_samples`, 9.5–9.8 there — for a fixture whose forecasts were already computed, and § 1 and § 3 now say what a user waits for instead. Measured with the branch running as host processes against the shared Postgres and the shared containers' own run; the same `/simulate` payload against the containerised ml-service is 145.4 ms median against the host process's 317.2, so inside the dev stack's containers the re-score would land near 200 ms — the claim is out by a factor of 20–40 either way, which is why no draw count was lowered and no cache added to chase it. **One finding recorded rather than fixed:** the same probe's diagnostic arm swaps for a player the *rating order* ranks higher without dominating him, and the displayed probability falls in 0/8 T20I, 1/8 ODI, 4/8 T20 and 6/8 TEST cases (worst −0.0805) — `docs/BUG_BACKLOG.md` § B-8. **Tests:** Go — the pinned selection path, every refusal, the constraint report, the client sending constraints only for a pinned eleven, and three scratch-database integration tests through the real handler; ml-service — the check reports the counts, a broken constraint comes back broken and unrepaired, and checking moves no probability; frontend — the delta arithmetic as pure functions, the Play-mode state machine (an edit that leaves ten players does not re-score; the delta is measured against the answer the change was made from), and the board, chips and delta rendering. `make check-all` green; **frontend coverage ratcheted to 80/80/78/71**, go-app 76.8 % and ml-service 93.62 % each rounding down to the existing threshold. `cricket_data` unchanged at 22,818 / 11,539,808 / 13,662 |
| P1-3 | ✅ **shipped** (2026-09-06) — the "why this player" card, on every selected player, showing what the selection consumed and nothing else. **The rule was applied, not asserted:** § 4 now carries the field-by-field map from each number on the card to the code that produced it, and the four fields it first sketched that are **omitted with their reasons** — a *trajectory* (the served state holds decayed accumulators as of one date, not a history, so the objective consumed no earlier value; form appears only as the decayed rates inside the selection rating and its standing), a *must-include* role (go-app puts a required id into the pool and sends ml-service an empty `must_include`, so the search never treats anyone as required and may leave him out — a "required" chip would name a constraint nothing applied), a *top-order anchor* role (`exp_bat_position` is read by the performance model, never by the objective), and an *interval on the beats-whom gap* (one objective evaluation per candidate swap yields a point estimate; the card says so instead of inventing one). The same list is the card component's doc-comment, so the rule travels with the code. **New on the wire:** `/xi/optimize` gains `selection_reasons` per selected player — `roles` (from `_Pool.is_keeper` / `is_bowler`, the same predicates the constraints and the `has_keeper` / `n_bowlers` features read), `selection_rating` (`rating_order_score`, extracted so `_greedy_seed` and the card cannot compute different composites), `rating_percentile` and `pool_size` (standing within the pool *as served*), and `best_alternative` (the best excluded pool player over exactly `_best_neighbour`'s single-swap neighbourhood, with the P(win) that swap costs) — and go-app carries it onto each `SelectedPlayer` as `selection_reason`, resolving the alternative's registry id to a player id and name. An alternative the pool cannot resolve is reported on the wire with its reason, never dropped (§8.7). `selection_roles` joins the H-24 contract, asserted from all three components. **Verified on the dev stack** (branch ml-service and go-api as host processes against the shared Postgres and run `20260906T083819Z-36689f80`, ratings through 2026-09-02): T20I India v Australia comes back `optimised true`, P(India) 0.7312, with a reason on all 22 players — pools of 26 and 24, percentiles 12 to 100, and every beats-whom gap positive (0.0011 to 0.1403), which is what a converged single-swap search should give; TEST Australia v England comes back `optimised false` with a reason on all 22, **no marginal value and no alternative anywhere**, percentiles 40.9 to 100 over pools of 18 and 23. **B-8's honest half:** the rating-ordered card says the ordering it shows is the selection's own composite and not the win model's ranking, and that swapping in a higher-rated player can move the displayed probability down — the sentence a user needs before acting on the order; P1-4 carries the other half. **Cost, measured rather than assumed:** the best-alternative sweep is **11.4 ms median** in-process on a 26-player T20I pool, beside the search's 383.8 ms and the marginal values' 1.7 ms, so about 46 ms on an Optimise (four win-objective calls); **Play mode's re-score is untouched**, because it makes no `/xi/optimize` call at all, and P1-2's 355–399 ms stands. **Glossary:** four new L-1 entries — `xi_role`, `selection_rating`, `rating_percentile`, `next_best_gap` — reachable from every labelled number on the card. **Tests:** ml-service — the composite is the order the rating-ordered pick uses, percentiles run 0 to 100 over the pool, roles agree with `is_bowling_option` and the keeper flag, the rating-ordered path scores no alternative, the best alternative equals a swap re-scored through the objective, no gap is negative on a converged search, an unpooled player gets no entry, and both `/xi/optimize` paths carry the block; go-app — the alternative resolves to a player id and name, an unresolvable one says so on the wire, the reasons come from the same round as the marginal values, Play mode carries none, the client maps the block on both objectives, and a scratch-database integration test through the real handler shows a rating-ordered payload carrying a reason per player with no alternative; frontend — the two card states, the rating-ordered one asserted to carry no marginal value and no beats-whom line even when handed one, the point-estimate sentence, the negative-marginal reading, and a control on every row. `make check-all` green; coverage gates never moved down — the frontend's statements gate ratcheted 78 -> 79, and **go-app's did not move, for a reason worth recording**: it measured 77.0 % locally and 76.9 % in CI, and the difference is exactly one statement in an unrelated package. `ctxReader.Read`'s cancellation branch is covered on a fast machine and not on the CI runner, because the test asserting it only asserts that `Fetch` errors, which it does whether or not the cancellation reaches that reader. The total sits on the 76.95 boundary, so that one statement decides the rounded figure. Diagnosed by diffing the CI run's own `coverage.out` artifact against a local profile — they differ in that one function and nothing else — and recorded as `docs/BUG_BACKLOG.md` § B-9; the threshold stays at main's 76, which is 76.9 rounded down. `cricket_data` unchanged at 22,818 / 11,539,808 / 13,662 |
| P1-4 | ✅ **shipped** (2026-09-06) — the sweep: every honesty surface, on every state of the Lab, checked against an inventory rather than a feeling. **The inventory** (in the PR, one row per number: what stands beside it, its range, its explainer key, its date) walked eight served states — T20I and ODI optimised, T20 and TEST rating-ordered, the three toss states, a must-include request, Play mode after a swap — and four refused ones. **What it changed on the surface:** (1) *ranges* — every point with a 10–90 range on the wire shows it in every state; the one point the stack serves without a range (wickets on the performance-quantiles path, an expectation with P(0/1/2+) and no quantiles) reads "(no range)" and no interval is derived from the pmf; the ranges are the simulator's as served, X-2's day/night gap recorded in the `innings_total` explainer's band rather than adjusted for; (2) *the not-optimised notice* — an `Alert` at the XI from `selection.note`, titled for the state it is in ("picked by rating" / "your eleven"), and a response with no note says so instead of inventing a reason; the win probability beside it is labelled "the display model's read of this rating-ordered eleven / the eleven you built, not the result of a search"; (3) *the date* — one `RatingsAsOf` component renders "ratings as of *date* · run *id*" on every served prediction and in the stale readiness notice; the nothing-loaded notice says there is no date to show; (4) *the explainers* — every labelled number opens an L-1 entry, including the delta's labels in Play mode, and the win probability's source and the forecast's source are shown **by name** with an entry each: five new glossary keys, `innings_total`, `win_probability_source_display`, `win_probability_source_simulator`, `forecast_source_simulator`, `forecast_source_performance_quantiles`, keyed `<field>_<value>` off the two source vocabularies that now sit in the H-24 contract (`win_probability_sources`, `forecast_sources`; two sides, go-app and the frontend, because go-app decides both); `forecast` reaches the frontend for the first time — `predictteam.Result` always carried it and the Lab never read it, so the TEST card's "no innings length" sentence was the surface's own and is now the wire's. **B-8's other half, on the surface only:** a searched board is ordered by the marginal value the response already carries and says so ("the objective's own ranking of this eleven, which is what a swap here is measured against"); the rating-ordered board says it is in rating order and not the win model's ranking; the hand-built board says nothing ranks it; and Play mode carries the sentence about which ordering the swap guarantee holds under — dominance on every rated axis, not the rating order the T20/TEST eleven is listed in. No model and no selection policy changed. **The must-include label is now true, and the check it names now exists on the Optimise path:** the input reads "added to the pool, checked after"; go-app's `selection.must_include` names, per side, how many were asked for and each one the selection left out (an id the pool cannot resolve still stops the prediction), so "0 of 1 in the eleven · Ashok Sharma left out" is on screen where before nothing was; the real fix — a lock the optimiser honours — is `docs/BUG_BACKLOG.md` § B-10, not this item. **One wire change beyond the surface:** a transport failure to ml-service was `500 INTERNAL` with a dial string for a message; it is now `502 ML_UNREACHABLE` with the endpoint, the reason and a hint (§8.7 — the dependency that is down is named, not this service blamed). **Two states the gate did not anticipate, added to § 3's clauses 4 and 6:** Play mode's `fixed` selection is a third selection state and the Lab was titling it "Rating-ordered 11"; and the range-less wickets point above. **Demonstrated on the dev stack** with branch go-api and ml-service host processes beside the shared containers (which were not restarted — and the shared ml-service image, built 16:52Z, predates P1-3's 17:55Z merge, so it serves no `selection_reasons`; the success states went through a branch ml-service on the shared run instead): every served state through `POST /api/predict/team-selection` on run `20260906T083819Z-36689f80`, ratings through 2026-09-02 — T20I India v Australia 73.1 % display / 53.5 % simulated, innings 176 (142–220) and 175 (141–215); ODI 63.9 %; T20 MI v CSK rating-ordered 58.0 % with the E5 note; TEST Australia v England rating-ordered 51.3 % with the H-17 note, `forecast.source performance_quantiles` and its note, no scorecard, 22 wickets points with no range; the three toss states 73.1 / 72.6 / 73.6 % naming their order; a TEST toss reported not honoured; must-include `[11877, 395]` reporting Ashok Sharma left out and Bumrah in; Play mode swapping AR Patel (marginal −2.9 pp) for Ashok Sharma re-scoring 70.0 % (−3.1 pp) with both constraint blocks met — and each of these on the branch frontend on `localhost:5174` against that go-api, with the explainer popover opened from "source: display model". **The four refusals, each on the wire and on screen:** `503 RATINGS_STALE` ("ratings run through 2026-09-02 (4 days old, limit 1)") from a branch ml-service with `XI_RATINGS_MAX_AGE_DAYS=1`, and the readiness notice before a request showing the same date through the same component; `503 XI_MODEL_UNAVAILABLE` from a branch ml-service with an empty `MODELS_DIR`, and the readiness notice saying no run is loaded and no date can be shown; `502 ML_UNREACHABLE` from a go-api pointed at a closed port; `400 FIXTURE_CROSS_GENDER` on the wire only — the picker cannot build one, which is the point. In every refusal the Lab shows the message, the hint, the remedy and the code, and neither a number nor a date. **Tests:** frontend — an "honesty surfaces" suite per state (optimised, rating-ordered, rating-ordered with no reason served, Play mode after a swap, the three toss states, the must-include outcomes, stale before a request, nothing loaded, cross-gender, unreachable) asserting the notice, the ranges, the date and the explainer links; `boardOrder` and `RatingsAsOf` unit tests; the source vocabularies asserted against the contract; go-app — the must-include report in both outcomes and its absence in Play mode, the unreachable client error, the source vocabularies; ml-service — the five entries pass the completeness gate. `make check-all` green; **frontend lines gate ratcheted 80 → 81** (measured 81.29 / 80.92 / 79.39 / 71.61); go-app measured 77.0 % locally and stays at 76 (B-9: CI reads 76.9); ml-service 93.64 % rounds to its 93. `cricket_data` unchanged at 22,818 / 11,539,808 / 13,662 |
| P2 | open — availability as **maintained lists, not a licensed feed**; D-12's retirement ledger is the first piece (§ 5); the freshness badge takes § 2.1's gaps (3) and (4) after P1-5 closes (1) and (2) |
| P3 | open — **no wedge chosen** (route (a), § 2): P0-3 was skipped, so no evidence for a choice exists; if one is ever made it is recorded as an unevidenced judgment call, never as validated |
