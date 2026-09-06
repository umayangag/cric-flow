# Product roadmap: from an honest prediction system to something people pay for

**Status: proposal.** Written 2026-09-03. This is a business document kept to the repo's
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
- **Interactive what-if at ~10 ms per scenario**: swap a player, change the venue, flip
  the opposition, and watch the probability, totals and scorecard move. No competitor
  consumer product offers this with honest uncertainty attached.
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
| P0-4 | **Freshness decision** — ✅ **answered as a decision** (2026-09-04; the write-up is § 2.1 below). With paid feeds out of scope there was no licensed feed to quote against, so the comparison collapsed and the answer was forced: the system ships **"as of last import", with the as-of date visible**, and refuses a live prediction against ratings past H-11's limit rather than answering from a squad that has moved on. The write-up records the decision, audits the freshness machinery that actually exists (and names what does not), measures the staleness this box lives with, and puts the **Cricsheet licence question beside the cost line**, where it belongs. X-2 later put a second thing there: the weather source is free **non-commercially only**, so the zero is conditional on the use staying non-commercial (§ 2.1) | **The recurring data cost is zero, and what is paid instead is staleness.** Measured 2026-09-04 on this box: served ratings run through 2026-09-02, 2 days against the 14-day limit, and the weekly cadence bounds that at eight or nine days in normal operation. The decision is made; the *surfacing* it implies — the as-of date on the prediction itself, not only on the operator tabs — is Phase 2 work (§ 5), and § 2.1 names the gaps without closing them |

Exit gate: a one-page positioning statement whose every claim the harness supports, a
chosen first wedge, and a cost line. If P0-1 lands far below market and P0-3 finds no
pull, the honest outcome is "internal tool, no business" — recorded, like any null.

**Where the gate stands (2026-09-04): awaiting a user decision.** P0-1 ran and did not
resolve the market question; P0-2 is deferred; P0-3 — the evidence the gate's "chosen
first wedge" was to rest on — is skipped; P0-4's answer was forced and is now written up
(§ 2.1); the cost line is zero by construction. The gate cannot close on evidence, and
there are two honest routes through it:

- **(a)** accept the prototype framing and record the exit gate as *"internal tool /
  prototype, no wedge chosen"* — the outcome this section already names as a legitimate
  recorded result; or
- **(b)** choose a wedge by judgment and record it **explicitly as unevidenced**, so that
  nothing built on it later reads as validated.

Neither is chosen here. That decision is the user's; this document records the routes,
not a preference, and § 10 carries the gate as open until the user records one.

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
says must always travel with it. (2) The as-of date is absent from the one user-facing
prediction surface except when the prediction fails. (3) The manifest cannot answer "what
date is this run's data?" without loading the run. (4) Two freshness rules with different
inputs and different thresholds coexist and can disagree — and today they do: `db_freshness`
reads **stale** overall (TEST's latest match is 8 days old, past its 7-day bucket) while
H-11 reads **fresh** (2 days of 14). Neither is wrong; they measure different things, and
nothing reconciles them or says which one a badge would mean. All four belong to Phase 2's
freshness badge (§ 5), not to this item, which is a decision and an audit.

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

The client-facing workbench, on the existing serving stack (go-app + ml-service already
answer every call this needs; the work is product engineering):

- Pool picker (search players, or start from a real squad), opposition pool, venue,
  date, format; **Optimise** returns the XI with win probability, simulated totals and
  scorecard.
- **Play mode**: add/remove/swap players on either side; every change re-scores live
  (~10 ms) and shows the delta; constraint chips (keeper, bowlers, must-include). The
  displayed probability is monotone under that swap **by construction**: upgrading a player
  never lowers it, in any format, measured at 0.0000 violations per fold. It used to move
  the wrong way 3–7 % of the time, and buying the guarantee cost display AUC in ODI and
  TEST (`docs/BUG_BACKLOG.md` § B-7) — a deliberate trade, made because this bullet is the
  product.
- **The "why this player" card** (§4) on every selected player.
- **Toss toggle** (bat first / bowl first / unknown) on the Lab and the Upcoming-match
  surfaces, wired to the `team1_bats_first` parameter the API already carries end to end;
  "unknown" is today's behaviour — the simulator marginalises over the toss — so the
  toggle exposes a choice the stack already makes, rather than adding one.
- Honesty built into the UI, not the footnotes: ranges always shown, the not-optimised
  notice where selection is rating-ordered (T20, TEST), "ratings as of <date>", the
  metric explainers (L-1) reused throughout.
- SaaS plumbing: accounts/auth, per-user saved scenarios, rate limits, usage metering,
  hosting with the run-manifest discipline in production, uptime monitoring.
  Freemium: N scenarios/day free; subscription for unlimited + saved squads + exports.
  **Out of scope under the standing constraint, until the user decides otherwise:**
  hosting, accounts and metering carry a real running cost, so Phase 1 is scoped as a
  **locally-run prototype** — the Team Lab on the dev stack (`make dev-up`), single-user,
  no accounts, no billing. The bullet stays because a product needs it; it is not being
  built.

Exit gate: strangers use it unassisted; retention over novelty measured (do users return
in week 3?); infra cost per active user known. **As written this gate is a user study**,
which the standing constraint rules out, and its cost-per-user line needs hosting that
is not in scope. Until the constraint is lifted the prototype's gate is the narrower
*"the operator can run the Lab end to end on the dev stack"* — which measures nothing
about retention or cost, and says so.

## 4. The "why this player" card (Phase 1, and the honesty rule for it)

Per selected player, assembled from existing computations: **marginal value** (P(win)
lost if replaced by an average player — the headline); **role in the XI** (which
constraint or balance they satisfy: keeper, fifth bowler, top-order anchor — from the
optimiser's constraint state); **expected contribution** (L2-B median and 10–90 range
for runs/wickets in THIS fixture); **form and standing** (rating percentile within the
pool, trajectory); **the beats-whom line** (against the best excluded alternative:
the marginal-value gap, with its uncertainty). The rule: the card shows only inputs the
objective actually consumed (H-2's spirit — explanations are the model's real factors,
never post-hoc stories), and where selection is rating-ordered the card says the simpler
truth: picked by rating, not by the win model.

## 5. Phase 2 — data operations as a product

Scheduled ingest (A-5's cadence, in production), squad/availability data as **maintained
lists, not a licensed feed** — a feed is a paid source and the standing constraint rules it
out, so availability is what operators and users record, and D-12's retirement ledger
(a user's flag, promoted to a fact only when held data corroborates it) is the first piece
of that list; the pool-accuracy problem is a product problem now — the freshness badge
everywhere (P0-4's decision, on every surface, closing the four gaps § 2.1 names, the
dateless prediction payload first), and the **public track record page**: every published prediction
scored after the match, cumulative calibration plotted, misses included. This page is
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
| The lab is a toy (week-3 retention fails) | Phase 1 exit gate measures it; pivot weight to 3a/3b, which sell to workflows, not curiosity |

## 9. Sequencing and effort

P0 (weeks, mostly not code — P0-1 is one harness PR) → P1 (the big build: months of
product engineering; the model layer is done) → P2 (ongoing ops, starts during P1) →
one wedge of P3 (months) → P4 (earned, not scheduled). Each phase gets item-by-item
kickoff prompts in the FOLLOW_UP_PROMPTS.md style when it starts; P0-1's prompt exists
in spirit already (the A-6 market-benchmark sketch).

Under the standing constraint the sequence reads differently. P0 is mostly not runnable
(§ 2: one item measured, one deferred, one skipped, one forced), and its exit gate waits
on the user. P1 is a **locally-run prototype**, not a hosted product: its months of
product engineering exclude the SaaS plumbing, whose hosting, accounts and metering are a
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
| P0-4 | ✅ **answered as a decision** (2026-09-04; write-up in § 2.1). The system ships **"as of last import" with the as-of date visible**, and refuses a live prediction past H-11's limit rather than answering from stale ratings; paid feeds were out of scope, so there was no feed to quote and the answer was forced. **Recurring data cost: zero. What is paid instead: staleness** — measured on this box, served ratings through 2026-09-02, **2 days** against the 14-day limit, with the database's own latest match on the same date (nothing lost between import and serving), and the weekly cadence bounding the age at eight or nine days. **Recorded beside the cost line: the Cricsheet licence is unresolved** — no licence is stated for the match archive, only the author's criteria (free, derivatives allowed, corrections reported, not resold), which this prototype's use sits inside; it must be settled with the project directly before anything ships or is sold ([config-and-data.md](config-and-data.md) § Data-source licence register). **The decision is answered; the surfacing it implies is not** — § 2.1's audit names four gaps (the prediction payload carries no as-of date or run id; the Upcoming-match tab shows the date only when the prediction would be refused; the run manifest records `cutoff` but not `ratings_through`; go-app's `db_freshness` buckets and H-11 are two unreconciled rules, disagreeing today) and fixes none: that is Phase 2's freshness badge (§ 5) |
| P0 exit gate | **awaiting a user decision** — route (a) "internal tool / prototype, no wedge chosen" or route (b) a wedge chosen by judgment and recorded as unevidenced; neither chosen here (§ 2) |
| P1 | open — gated on P0; scoped as a **locally-run prototype** under the standing constraint (no hosting, accounts or metering; § 3, § 9) |
| P2 | open — availability as **maintained lists, not a licensed feed**; D-12's retirement ledger is the first piece (§ 5) |
| P3 | open — wedge cannot be chosen on evidence (P0-3 skipped); waits on the user's route through the P0 exit gate |
