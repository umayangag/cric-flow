# Product roadmap: from an honest prediction system to something people pay for

**Status: proposal.** Written 2026-09-03. This is a business document kept to the repo's
evidentiary standard: every capability claim below is either in the measured record
([ML_PIPELINE_REARCHITECTURE_PLAN.md](ML_PIPELINE_REARCHITECTURE_PLAN.md),
[FOLLOW_UP_PLAN.md](FOLLOW_UP_PLAN.md)) or is marked as an assumption to validate. Phases
gate on evidence, the way the migration's PRs did.

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
- **Unbenchmarked against the market.** We do not yet know whether these probabilities
  match betting-market accuracy on our own match population. The first sophisticated
  customer asks this; Phase 0 answers it before marketing writes a word.
- **Entrenched B2B incumbents** (ball-tracking-data analytics firms) own the franchise
  market's top end. The open flank is women's cricket, associate nations and emerging
  leagues, where public ball-by-ball data is the same data everyone has.
- **Freshness is the real operating cost.** Cricsheet lags matches by days. A product
  whose ratings are stale is H-11 with a paying customer attached; near-real-time needs
  a licensed data feed (recurring cost) or honest "as of <date>" positioning.
- **Regulatory adjacency.** A tool that optimises fantasy teams or resembles betting
  advice carries real compliance weight in the biggest cricket market (India). Phase 0
  scopes this before any fantasy pivot.

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
| P0-1 | **Market benchmark.** Join historical closing odds (a purchasable dataset) onto our matches; score them in the harness beside the display model, per format | The honest sentence marketing may use. At market accuracy → "market-grade, plus interactivity and explanations". Below → the pitch is the lab, and we say so |
| P0-2 | **Legal scan**: fantasy-adjacency and prediction-tool rules in target jurisdictions (India foremost), stats/name usage (facts are generally fair; images/logos are not — budget for licensing or ship without), terms for "not betting advice" | Which wedges are open at all, and the disclaimer architecture |
| P0-3 | **Wedge interviews**: 10–15 conversations — serious fantasy players, one associate-nation or women's-team analyst, one emerging-league operator | Which Phase 3 wedge is pulled, not pushed |
| P0-4 | **Freshness decision**: quote licensed feeds vs ship "as of last import" with the date always visible | The recurring-cost line in the model |

Exit gate: a one-page positioning statement whose every claim the harness supports, a
chosen first wedge, and a cost line. If P0-1 lands far below market and P0-3 finds no
pull, the honest outcome is "internal tool, no business" — recorded, like any null.

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

Exit gate: strangers use it unassisted; retention over novelty measured (do users return
in week 3?); infra cost per active user known.

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

Scheduled ingest (A-5's cadence, in production), squad/availability data (licensed feed
or maintained lists — the pool-accuracy problem is a product problem now), the freshness
badge everywhere, and the **public track record page**: every published prediction
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

## 10. Record of outcomes

| id | status |
|---|---|
| P0-1 | open |
| P0-2 | open |
| P0-3 | open |
| P0-4 | open |
| P1 | open — gated on P0 |
| P2 | open |
| P3 | open — wedge undecided until P0-3 |
