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
  a licensed data feed (recurring cost) or honest "as of <date>" positioning.
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
| P0-4 | **Freshness decision** — **answer forced, item simplified** (standing constraint). With paid feeds out of scope there is no licensed feed to quote against, so the comparison collapses: the system ships **"as of last import" with the date always visible** — the H-11 staleness refusal and the `ratings_through` surfaces already do the showing. P0-4 survives as the **write-up of that forced decision** — the cadence it implies, what the lag costs per format, where and how the date is shown — rather than as a comparison. **Not done**: the write-up is still its own item | The recurring-cost line is zero by construction; the write-up records what that buys and what it forgoes |

Exit gate: a one-page positioning statement whose every claim the harness supports, a
chosen first wedge, and a cost line. If P0-1 lands far below market and P0-3 finds no
pull, the honest outcome is "internal tool, no business" — recorded, like any null.

**Where the gate stands (2026-09-04): awaiting a user decision.** P0-1 ran and did not
resolve the market question; P0-2 is deferred; P0-3 — the evidence the gate's "chosen
first wedge" was to rest on — is skipped; P0-4's answer is forced; the cost line is zero
by construction. The gate cannot close on evidence, and there are two honest routes
through it:

- **(a)** accept the prototype framing and record the exit gate as *"internal tool /
  prototype, no wedge chosen"* — the outcome this section already names as a legitimate
  recorded result; or
- **(b)** choose a wedge by judgment and record it **explicitly as unevidenced**, so that
  nothing built on it later reads as validated.

Neither is chosen here. That decision is the user's; this document records the routes,
not a preference, and § 10 carries the gate as open until the user records one.

## 3. Phase 1 — the Team Lab (the product core)

The client-facing workbench, on the existing serving stack (go-app + ml-service already
answer every call this needs; the work is product engineering):

- Pool picker (search players, or start from a real squad), opposition pool, venue,
  date, format; **Optimise** returns the XI with win probability, simulated totals and
  scorecard.
- **Play mode**: add/remove/swap players on either side; every change re-scores live
  (~10 ms) and shows the delta; constraint chips (keeper, bowlers, must-include).
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
everywhere (P0-4's forced answer, on every surface), and the **public track record page**: every published prediction
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
| Freshness cost eats the margin | P0-4 decides feed-vs-honest-lag before pricing exists |
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
| P0-4 | open — **answer forced** by the constraint (no feed to quote; ship "as of last import" with the date visible); the write-up of that decision is still to be done as its own item (§ 2) |
| P0 exit gate | **awaiting a user decision** — route (a) "internal tool / prototype, no wedge chosen" or route (b) a wedge chosen by judgment and recorded as unevidenced; neither chosen here (§ 2) |
| P1 | open — gated on P0; scoped as a **locally-run prototype** under the standing constraint (no hosting, accounts or metering; § 3, § 9) |
| P2 | open — availability as **maintained lists, not a licensed feed**; D-12's retirement ledger is the first piece (§ 5) |
| P3 | open — wedge cannot be chosen on evidence (P0-3 skipped); waits on the user's route through the P0 exit gate |
