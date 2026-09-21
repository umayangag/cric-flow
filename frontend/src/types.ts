// --- Upcoming match prediction ---

/** A 10-90 interval shown beside a point. */
export type PredictValueRange = {
  p10: number;
  p90: number;
};

/**
 * One player of a selected XI.
 *
 * Every point has a range beside it, because the models are distributional: a median with
 * no interval reads as a promise the model never made. `marginal_value` is what the XI
 * loses without this player, absent when nothing was maximised; `spread_share` is his share
 * of the innings total's variance, present only where the simulator ran.
 */
/**
 * The pool player the objective would most like instead of a selected one, and the win
 * probability that swap would cost (P1-3).
 *
 * A point estimate with no interval, because the computation behind it produces none. At or
 * below zero it means the search stopped on its evaluation budget, not at a local optimum.
 */
export type PredictBestAlternative = {
  player_id: number;
  player_name: string;
  win_probability_gap: number;
};

/**
 * Why one selected player is in the eleven, in the terms the selection itself used (P1-3).
 *
 * Every field is a value the selection consumed: the constraint predicates it evaluated
 * (`roles`), the composite it ordered the pool by (`selection_rating` and its standing in
 * that pool), and the swap candidates it scored (`best_alternative`). Absent entirely in
 * Play mode, where the caller built the eleven and nothing selected it; `best_alternative`
 * is absent on a rating-ordered XI, where nothing was maximised.
 */
export type PredictSelectionReason = {
  roles: SelectionRole[];
  selection_rating: number;
  rating_percentile: number;
  pool_size: number;
  best_alternative?: PredictBestAlternative;
  /** Why the objective could name no alternative, where it was asked and could not. */
  best_alternative_note?: string;
};

export type PredictTeamSelectedPlayer = {
  player_id: number;
  player_name: string;
  runs: number;
  runs_range?: PredictValueRange;
  balls?: number;
  balls_range?: PredictValueRange;
  wickets: number;
  wickets_range?: PredictValueRange;
  runs_conceded: number;
  runs_conceded_range?: PredictValueRange;
  economy?: number;
  marginal_value?: number;
  spread_share?: number;
  /** What the selection read about this player (P1-3); absent where nothing selected him. */
  selection_reason?: PredictSelectionReason;
};

/**
 * How the XIs were chosen. `optimised` is false where the win objective does not rank
 * (H-17: TEST), and every surface showing such an XI has to say so.
 *
 * `fixed` is Play mode (P1-2): the caller built the eleven and the API scored it, so
 * nothing was searched for and no player carries a marginal value. It is go-app's own
 * value — ml-service is told which players to score and never asked how they were chosen.
 *
 * The three values are declared in contracts/ops-console.contract.json and asserted against
 * it by `opsContract.test.ts` (H-24). They became a shared vocabulary with the prediction
 * record (P2-3), which stores the objective as a column: `fixed` is what tells the track
 * record that a stored answer is a scenario to be listed and never scored.
 */
export const SELECTION_OBJECTIVES = ['win', 'ratings', 'fixed'] as const;
export type SelectionObjective = (typeof SELECTION_OBJECTIVES)[number];

export type PredictSelectionSummary = {
  objective: SelectionObjective;
  optimised: boolean;
  note?: string;
  /**
   * What became of the must-include ids: present where any were asked for on a selected
   * eleven. Since B-10 the ids are a lock the selection honours, so this is the
   * postcondition — an ordinary answer holds every one of them, and a named "left out"
   * player means a lock the stack accepted was not honoured.
   */
  must_include?: PredictMustIncludeReport;
};

/** One side's must-include ids against the eleven that was selected. */
export type PredictMustIncludeStatus = {
  requested: number;
  /** Always an array: "checked, none missing" is an empty list, not an absent one. */
  missing: PredictMissingPlayer[];
};

export type PredictMustIncludeReport = {
  team1: PredictMustIncludeStatus;
  team2: PredictMustIncludeStatus;
};

/**
 * Which model produced the headline win probability, exactly as the wire spells it.
 *
 * Declared in contracts/ops-console.contract.json and asserted against it by
 * `opsContract.test.ts` (H-24). The Lab names the source beside the number and opens its
 * explainer from the name (glossary key `win_probability_source_<value>`), so a value the UI
 * cannot spell would be a probability shown with no model behind it (P1-4).
 */
export const WIN_PROBABILITY_SOURCES = ['display', 'simulator'] as const;
export type WinProbabilitySource = (typeof WIN_PROBABILITY_SOURCES)[number];

/**
 * Which model produced the per-player numbers, exactly as the wire spells it (H-24, P1-4).
 *
 * `simulator` is one set of drawn whole matches, so the lines and extras sum to the innings
 * total; `performance_quantiles` is L2-B's per-player distributions reported directly, on a
 * format with no innings length, where there is no total to sum to.
 */
export const FORECAST_SOURCES = ['simulator', 'performance_quantiles'] as const;
export type ForecastSource = (typeof FORECAST_SOURCES)[number];

/**
 * Which model produced the per-player numbers, and — where that is not the simulator —
 * why. A fallback that changes which model answered is named on the wire (§8.7).
 */
export type PredictForecastSummary = {
  source: ForecastSource;
  note?: string;
};

/**
 * The headline win probability and which model produced it. The other model's answer is
 * reported beside it, never blended with it.
 */
export type PredictWinProbability = {
  team1: number;
  source: WinProbabilitySource;
  simulated?: number;
  predicted_winner: string;
};

/** One simulated innings: the median-band total the scorecard sums to, and its 10-90 range. */
export type PredictInningsTotal = {
  total: number;
  extras: number;
  p10: number;
  median: number;
  p90: number;
};

/**
 * Which of the two readings a prediction's probabilities are, exactly as the wire spells
 * it (H-24, GO-07).
 *
 * `marginalised` averages both batting orders and is what the optimiser maximises and what
 * the run manifest and the walk-forward report score; `toss_aware` reads the batting order
 * the caller named. They are two different numbers — 0.04 apart on average in TEST, 0.14 at
 * most — so which one is on screen has to be said, not inferred.
 */
export const TOSS_READINGS = ['toss_aware', 'marginalised'] as const;
export type TossReading = (typeof TOSS_READINGS)[number];

/**
 * Which batting order the numbers were read at (P1-1, GO-07).
 *
 * `team1_bats_first` is null where the toss was unknown and both orders were read, which is
 * the default. `reading` names the resulting quantity, and is derived from what each model
 * reported having done rather than from the request — a model that answered a different
 * batting order from the one asked for is refused, not labelled (§8.7). `note` is present
 * on a toss-aware answer and says what in it still did not read the toss.
 */
export type PredictTossSummary = {
  team1_bats_first: boolean | null;
  reading: TossReading;
  note?: string;
};

/** The simulated match. Absent for a format with no innings length. */
export type PredictScorecard = {
  samples: number;
  toss_marginalised: boolean;
  team1_innings: PredictInningsTotal;
  team2_innings: PredictInningsTotal;
};

/**
 * What a Stop achieved, which is not the same as what it attempted.
 *
 * `training_stopped` names the steps ml-service confirmed it killed — a Stop that ended a
 * twelve-minute retrain and a Stop that found nothing running are different events, and the
 * console could not previously tell them apart. `status: 'partially_cancelled'` (with 502)
 * means the run was cancelled here but the training process could not be confirmed stopped,
 * which used to be reported as a plain success while `ml.xi.retrain` kept going (D-11).
 */
export type PipelineStopResult = {
  status?: 'cancelled' | 'partially_cancelled';
  cancelled?: number;
  plan_stopped?: boolean;
  training_stopped?: string[];
  error?: string;
};

/**
 * The gender half of a team's identity, exactly as the wire spells it.
 *
 * Declared in contracts/ops-console.contract.json and asserted against it by
 * `opsContract.test.ts` (H-24): go-app writes these values, ml-service matches on them, and
 * this is the third component that has to agree. Never hand-type one of these strings
 * elsewhere in the UI — a side's label comes from the backend as `display_name`.
 */
export const TEAM_GENDERS = ['male', 'female'] as const;
export type TeamGender = (typeof TEAM_GENDERS)[number];

/**
 * One side of a fixture: the club id a prediction request is made with, plus the name and
 * gender that make it one team.
 *
 * A name alone is not a team — 130 of the 394 names in the dataset are used by both a men's
 * and a women's side — so the picker offers sides and sends `club_id` (D-10).
 */
export type TeamSideOption = {
  club_id: number;
  name: string;
  gender: TeamGender;
  display_name: string;
};

/**
 * How a side's candidate pool was chosen, exactly as the wire spells it.
 *
 * Declared in contracts/ops-console.contract.json and asserted against it by
 * `opsContract.test.ts` (H-24). The pool used to be all-time and unstated, which is how
 * the Upcoming-match tab came to offer players who retired a decade ago (D-12); a source
 * the UI cannot name would put that silence back.
 */
export const POOL_SOURCES = ['recency_window', 'all_time', 'manual'] as const;
export type PoolSource = (typeof POOL_SOURCES)[number];

/**
 * Why a candidate the window offered is not in the pool an XI was chosen out of.
 *
 * Two come from the retirement ledger and are the user's own to undo. `both_sides` is not
 * the ledger at all: the fixture's other side holds the same player and nobody plays both
 * elevens, so the side that played him less recently lost him (GO-04). It is shown for the
 * same reason the other two are -- a filter that is not shown is indistinguishable from no
 * filter (§8.7) -- but there is no flag behind it to withdraw.
 */
export const POOL_EXCLUSION_REASONS = ['user_flagged', 'retired', 'both_sides'] as const;
export type PoolExclusionReason = (typeof POOL_EXCLUSION_REASONS)[number];

/**
 * The constraint state a "why this player" card may name, exactly as the wire spells it.
 *
 * Declared in contracts/ops-console.contract.json and asserted against it by
 * `opsContract.test.ts` (H-24). ml-service computes these from the predicates its optimiser
 * evaluates; a role the UI cannot name would be a constraint the objective really did read
 * and the card silently dropped (P1-3).
 */
export const SELECTION_ROLES = ['keeper', 'bowling_option'] as const;
export type SelectionRole = (typeof SELECTION_ROLES)[number];

/** One candidate the ledger kept out of the pool, with the reason a user can undo. */
export type PoolExcludedCandidate = {
  player_id: number;
  player_name: string;
  /** YYYY-MM-DD; absent where he never played for this club in this format. */
  last_played?: string;
  reason: PoolExclusionReason;
  /** The criterion's evidence, where a criterion corroborated the flag. */
  detail?: string;
};

/** Which candidates an XI was chosen out of, and who was left out (D-12). */
export type PoolSummary = {
  source: PoolSource;
  /** The window applied, in months; absent on an all-time or manual pool. */
  window_months?: number;
  /** The first match date the window accepted, YYYY-MM-DD. */
  since?: string;
  size: number;
  retired_excluded: number;
  excluded?: PoolExcludedCandidate[];
};

/** One player on the candidate list a manual pool is ticked out of. */
export type PoolCandidate = {
  player_id: number;
  player_name: string;
  is_wicket_keeper: boolean;
  last_played?: string;
  /** True where the ledger is keeping him out of the default pool. */
  excluded: boolean;
  reason?: PoolExclusionReason;
  detail?: string;
};

/** GET /api/options/candidates: the list, and the scope it was drawn from. */
export type CandidatesResponse = {
  side: TeamSideOption;
  pool: PoolSummary;
  candidates: PoolCandidate[];
};

/**
 * What flagging or un-flagging a player did.
 *
 * `promoted` is the honest part: a claim that corroborated nothing hides the player from
 * this user's pools and from nobody else's, and the response says so rather than letting
 * the user believe they changed a fact about the player.
 */
export type RetirementStatus = {
  player_id: number;
  flagged: boolean;
  promoted: boolean;
  criterion?: string;
  detail?: string;
  /** Criteria whose evidence does not exist yet. */
  unchecked?: string[];
  notes?: string[];
  /** On an un-flag: whether there was a claim, and whether withdrawing it lowered the fact. */
  existed?: boolean;
  demoted?: boolean;
};

/** One side's pool scope on a prediction request: the window, or a hand-picked subset. */
export type PoolRequest = {
  window_months?: number;
  all_time?: boolean;
  /** The manual pick. When present it is the pool; the window and ledger do not apply. */
  players?: number[];
};

/**
 * Which rating state a prediction was served from (P1-5): the run its models and ratings
 * were loaded from, and the last match date those ratings include. Both are always
 * present — a prediction without its date is refused by go-app, never served blank.
 */
export type PredictServedRatings = {
  /** YYYY-MM-DD: the served ratings include every match through this date and none after. */
  ratings_through: string;
  run_id: string;
};

/** A must-include player an eleven does not hold, named rather than left as an id. */
export type PredictMissingPlayer = {
  player_id: number;
  player_name: string;
};

/**
 * One hand-built eleven measured against its constraints (P1-2).
 *
 * The counts are ml-service's, where "a bowling option" and "a keeper" are defined, so a
 * chip reading "4 of 5 bowlers" counts what the optimiser would have counted. Nothing here
 * changed the eleven: a broken constraint is shown broken and the eleven is scored as built.
 */
export type PredictConstraintStatus = {
  size: number;
  bowlers: number;
  has_keeper: boolean;
  missing_must_include?: PredictMissingPlayer[];
  met: boolean;
};

/** What was asked of both elevens, and how each one measures up. */
export type PredictConstraintReport = {
  team_size: number;
  min_bowlers: number;
  require_keeper: boolean;
  team1: PredictConstraintStatus;
  team2: PredictConstraintStatus;
};

/**
 * Which venue the answer was produced at (GO-08).
 *
 * `resolved` is false only where no venue was named -- the fixture was read without one,
 * and `note` says so. A venue that was named and could not be found is a refusal
 * (`VENUE_NOT_FOUND`), not an unresolved answer.
 */
export type PredictVenueSummary = {
  resolved: boolean;
  venue_id?: number;
  name?: string;
  note?: string;
};

export type PredictTeamSelectionResponse = PredictServedRatings & {
  /** The sides that were actually scored, echoed back whether or not the request was clear. */
  team1_side: TeamSideOption;
  team2_side: TeamSideOption;
  team1: PredictTeamSelectedPlayer[];
  team2: PredictTeamSelectedPlayer[];
  selection: PredictSelectionSummary;
  /** Which model produced the per-player numbers, and why where it is not the simulator. */
  forecast: PredictForecastSummary;
  win_probability: PredictWinProbability;
  /** Which batting order the numbers were produced under, and whether a named one was used. */
  toss: PredictTossSummary;
  /** Which venue every model read, or that none was named. */
  venue: PredictVenueSummary;
  scorecard?: PredictScorecard;
  /** Which candidates each XI was chosen out of, and who the ledger excluded (D-12). */
  team1_pool: PoolSummary;
  team2_pool: PoolSummary;
  /**
   * Whether each hand-built eleven meets what was asked of it (P1-2). Present only in
   * Play mode: an eleven the optimiser chose was chosen under the constraints, while one
   * a user built is checked against them and never repaired.
   */
  constraints?: PredictConstraintReport;
  /**
   * What became of the attempt to file this answer in the prediction record (P2-3).
   *
   * Always present on a served prediction. `stored: false` means the answer is correct and
   * was served, and that this one will not be on the track record — a substitution the Lab
   * shows rather than leaving in a server log (§8.7).
   */
  record: PredictRecordBlock;
};

/** What an answer says about its own filing in the prediction record (P2-3). */
export type PredictRecordBlock = {
  stored: boolean;
  /** The stored row's id, and when it was issued. Absent where nothing was stored. */
  id?: string;
  issued_at?: string;
  /** Why it was not stored, in the store's own words. Present only on a failure. */
  reason?: string;
};

/** ml-service GET /health, via the go-app proxy. */
export type HealthResponse = {
  status: string;
  models_dir: string;
  loaded: boolean;
  /** The run being served (H-16), or null when nothing loaded. */
  run_id: string | null;
  loaded_xi_formats?: string[];
  loaded_performance_formats?: string[];
  /** H-11's verdict on the loaded ratings, not just their date. */
  ratings?: RatingsFreshness | null;
  /** Why nothing is loaded, when artifacts on disk were refused (D-6). */
  error?: string | null;
};

/**
 * How the one freshness verdict is spelled, exactly as the wire spells it (H-24, P2-1).
 *
 * Declared in contracts/ops-console.contract.json and asserted against it by
 * `opsContract.test.ts`. There is one verdict in this system — H-11's, computed by
 * ml-service against `ml.ratings_max_age_days` — and these words are how go-app's
 * assembled `freshness` object reports it. The UI holds no threshold and no second rule:
 * `db_freshness` reading *stale* while H-11 read *fresh* on the same box is what a private
 * vocabulary cost (§ 2.1 gap (4)).
 */
export const FRESHNESS_STATUSES = ['fresh', 'stale', 'not_loaded', 'unknown'] as const;
export type FreshnessStatus = (typeof FRESHNESS_STATUSES)[number];

/**
 * The one state every stored prediction is in on the track record (H-24, P2-4), and the
 * simulator population its ranges belong to -- never pooled (B-12). Both are declared in
 * contracts/ops-console.contract.json and asserted against it; go-app computes them.
 */
export const PREDICTION_STATES = [
  'scenario',
  'superseded',
  'unresolved',
  'no_result',
  'post_hoc',
  'scored',
] as const;
export type PredictionState = (typeof PREDICTION_STATES)[number];
export const SIMULATOR_POPULATIONS = [
  'with_shared_factor',
  'without_shared_factor',
  'unknown',
  'not_simulated',
] as const;
export type SimulatorPopulation = (typeof SIMULATOR_POPULATIONS)[number];
/** The L-1 keys the track record labels its numbers under; the contract holds the same list. */
export const TRACK_RECORD_METRIC_KEYS = [
  'brier',
  'record_base_rate_brier',
  'reliability',
  'coverage_80',
  'eleven_overlap',
] as const;

/**
 * The auction record (P3-1).
 *
 * A player is in exactly one of three states, and the vocabulary is the contract's: go-app
 * writes it, the database's CHECK constraint holds it and this tab renders it. A state the
 * UI could not spell would be a player on the wire and missing from every count on screen.
 */
export const AUCTION_PLAYER_STATES = ['available', 'sold', 'unsold'] as const;
export type AuctionPlayerState = (typeof AUCTION_PLAYER_STATES)[number];

/** The L-1 keys the Auction tab labels its numbers under; the contract holds the same list. */
export const AUCTION_METRIC_KEYS = [
  'auction_open_slots',
  'auction_available_by_role',
  'auction_projected_output',
  'auction_projected_total',
  'interval_source_l2b_quantiles',
  'interval_source_simulator_draws',
  'spread_share',
] as const;

/**
 * Where an interval on a projection came from (H-24, P3-2).
 *
 * Two intervals sit side by side on one row and they are different populations with
 * different evidence: L2-B's quantile heads are at nominal coverage on the harness, and the
 * simulator's drawn totals are B-11's open defect. A reader who could not tell them apart
 * would read one's evidence onto the other, so every interval names its source and opens
 * its own explainer.
 */
export const AUCTION_INTERVAL_SOURCES = ['l2b_quantiles', 'simulator_draws'] as const;
export type AuctionIntervalSource = (typeof AUCTION_INTERVAL_SOURCES)[number];

/**
 * What the served rating vectors say about one listed player.
 *
 * `known: false` is the served state having never seen him — his `roles` are then empty
 * and mean nothing, and the surface must show him as unknown rather than as a batter. A
 * known player with no roles is a batter *by elimination*: the system has measured no
 * other role vocabulary, and the glossary entry says so.
 *
 * The whole object is absent where the role read was refused, which is a third thing again
 * — the model was never asked.
 */
export type AuctionPlayerRoles = {
  known: boolean;
  roles: SelectionRole[];
};

/** One player on an auction list, as the record holds him. */
export type AuctionListedPlayer = {
  player_id: number;
  player_name: string;
  state: AuctionPlayerState;
  /** The buying franchise as the operator typed it; present on a sold row only. */
  buyer_name?: string;
  /** That buyer as a club id, where the buyer is a side this database knows. */
  buyer_club_id?: number;
  /** What was paid, in the auction room's own unit. No currency is implied. */
  price?: number;
  state_changed_at: string;
  roles?: AuctionPlayerRoles;
};

/** The side an auction is filling a squad for. */
export type AuctionBuyer = { club_id: number; name: string };

/** The auction as the operator entered it, and nothing derived. */
export type AuctionRecord = {
  id: string;
  name: string;
  format: string;
  created_at: string;
  buyer: AuctionBuyer;
  venue_ids: number[];
  squad_size: number;
  min_bowlers: number;
  require_keeper: boolean;
  players: AuctionListedPlayer[];
  /**
   * The projection's named assumptions (P3-2), on the record so every later item reads
   * one list. `likely_xi` is empty until the operator names it; `opposition` is absent
   * until they do, because a projection against no one is refused rather than made
   * against a silently neutral side.
   */
  likely_xi: AuctionProjectionPlayer[];
  opposition?: AuctionOppositionBlock;
};

/** One auction on the index an operator finds theirs from after a reload. */
export type AuctionSummary = {
  id: string;
  name: string;
  format: string;
  created_at: string;
  buyer: AuctionBuyer;
  squad_size: number;
};

/** What the buyer has bought. */
export type AuctionSquad = {
  size: number;
  player_ids: number[];
  players: AuctionListedPlayer[];
};

/** The open places read through the eleven's constraints; absent when the roles were refused. */
export type AuctionSlotsByRole = {
  keepers: number;
  bowling_options: number;
  keeper_needed: boolean;
  min_bowlers: number;
  bowling_options_short: number;
  unknown_roles: number;
};

export type AuctionSlots = {
  squad_size: number;
  filled: number;
  open: number;
  by_role?: AuctionSlotsByRole;
};

/**
 * The remaining pool by role.
 *
 * `keepers` and `bowling_options` overlap: they are two independent predicates and not a
 * partition, so a keeper who also bowls is in both. Only `batters` and `unknown` are
 * exclusive of everything else.
 */
export type AuctionDistribution = {
  available: number;
  keepers: number;
  bowling_options: number;
  batters: number;
  unknown: number;
};

/**
 * Whether the role read happened, and either what served it or why it did not (§8.7).
 *
 * A refused read leaves the record whole — it is facts the operator typed and needs no
 * model to be read back — and every number that comes off the model absent, with the code
 * ml-service refused under.
 */
export type AuctionRolesBlock = {
  available: boolean;
  run_id?: string;
  ratings_through?: string;
  code?: string;
  message?: string;
  hint?: string;
};

/** An auction as it now stands: every read and every write answers with this. */
export type AuctionResponse = {
  auction: AuctionRecord;
  squad: AuctionSquad;
  slots: AuctionSlots;
  distribution?: AuctionDistribution;
  roles: AuctionRolesBlock;
};

// --- The projection (P3-2) ---
//
// A projection is conditional on three things the operator named and not one of them is a
// fact: the eleven a candidate would join, the opposition it would face, and the grounds.
// All three come back on every answer, so a projection on screen says which guess it was
// made for. Nothing here carries a win probability or a marginal value: the module is
// valuation and projection and never XI-picking (plan §8.8).

/** One named player on a projection's assumptions. */
export type AuctionProjectionPlayer = { player_id: number; player_name: string };

/** The side a projection is against, as the operator named it. */
export type AuctionOppositionBlock = {
  club_id: number;
  name: string;
  players: AuctionProjectionPlayer[];
};

export type AuctionGroundRef = { venue_id: number; venue_name: string };

/**
 * A quantity's 10-50-90, with the source of the interval named.
 *
 * The values are exactly as served: no client-side widening, narrowing or day/night
 * adjustment, and no range is hidden.
 */
export type AuctionQuantiles = {
  q10: number;
  median: number;
  q90: number;
  interval_source: AuctionIntervalSource;
};

/**
 * The wicket count: an expectation and three probabilities, and deliberately no interval.
 * L2-B produces no quantile heads for wickets on this path, and one derived from the
 * probabilities would be an interval the model never made.
 */
export type AuctionWicketDistribution = {
  expected: number;
  p0: number;
  p1: number;
  p2_plus: number;
  note: string;
};

export type AuctionCandidateForecast = {
  runs: AuctionQuantiles;
  balls_faced: AuctionQuantiles;
  runs_conceded: AuctionQuantiles;
  wickets: AuctionWicketDistribution;
  innings_marginalised: boolean;
};

/** The ground as the model read it, and whether it therefore read neutral (§8.7). */
export type AuctionGroundContext = {
  bat_first_rate: number;
  matches: number;
  neutral: boolean;
  note?: string;
};

export type AuctionElevenTotal = {
  total: AuctionQuantiles;
  spread_share: number;
  samples: number;
  shared_factor: boolean;
};

export type AuctionGroundProjection = {
  venue_id: number;
  venue_name: string;
  ground: AuctionGroundContext;
  candidate: AuctionCandidateForecast;
  eleven_total: AuctionElevenTotal;
  toss_marginalised: boolean;
};

export type AuctionVenueWeight = { venue_id: number; weight: number };

/** The eleven's total over the venue mix, inverted from the simulator's pooled draws. */
export type AuctionMixture = {
  total: AuctionQuantiles;
  weights: AuctionVenueWeight[];
  note: string;
};

/**
 * What one interval source is, and what is open against it.
 *
 * No metric key: this surface spells the two L-1 keys itself, as literals asserted against
 * the contract (H-24), so a source it could not name is a failing test rather than an
 * interval shown with no explainer behind it.
 */
export type AuctionIntervalSourceBlock = {
  source: AuctionIntervalSource;
  label: string;
  caveats?: string[];
  note: string;
};

/** The toss a projection was made under, in the buyer's terms. */
export type AuctionProjectionToss =
  'unknown' | 'candidate_eleven_bats_first' | 'candidate_eleven_chases';

export type AuctionAssumptionsBlock = {
  eleven: AuctionProjectionPlayer[];
  opposition: AuctionOppositionBlock;
  grounds: AuctionGroundRef[];
  toss: AuctionProjectionToss;
  format: string;
  what_a_ground_changes: string;
  not_xi_picking: string;
};

export type AuctionProjection = {
  auction_id: string;
  candidate: AuctionProjectionPlayer;
  assumptions: AuctionAssumptionsBlock;
  grounds: AuctionGroundProjection[];
  mixture?: AuctionMixture;
  intervals: AuctionIntervalSourceBlock[];
  served_ratings: PredictServedRatings;
};

/** A side's last recorded eleven, offered as a starting point for the opposition. */
export type AuctionOppositionSuggestion = {
  club_id: number;
  name: string;
  players: AuctionProjectionPlayer[];
  from_match: { match_date: string; event_name?: string; venue_name?: string };
  note: string;
};

/** One player a cross-club name search found. */
export type PlayerSearchResult = {
  player_id: number;
  player_name: string;
  /** `player.is_wicket_keeper` — the database's name-set flag, and not the model's role. */
  is_wicket_keeper: boolean;
  last_played?: string;
  clubs: string[];
  formats: string[];
  excluded: boolean;
  reason?: PoolExclusionReason;
  detail?: string;
};

/** A Brier with the base rate beside it, over n scored predictions. Null over none. */
export type TrackRecordWinScore = {
  n: number;
  brier: number | null;
  base_rate: number | null;
  base_rate_brier: number | null;
};

export type TrackRecordReliabilityBin = {
  lo: number;
  hi: number;
  n: number;
  predicted: number;
  observed: number;
};

export type TrackRecordCoverageScore = {
  n: number;
  covered: number;
  coverage: number | null;
};

/** One (format, population) row; there is no pooled row across populations. */
export type TrackRecordCoverageRow = {
  format: string;
  population: SimulatorPopulation;
  n_predictions: number;
  first_innings: TrackRecordCoverageScore;
  chase: TrackRecordCoverageScore;
};

export type TrackRecordRange = { p10: number; p90: number };

export type TrackRecordEntry = {
  id: string;
  issued_at: string;
  run_id: string;
  ratings_through: string;
  format: string;
  gender: string;
  match_date: string;
  team1: { id: number; name: string };
  team2: { id: number; name: string };
  objective: SelectionObjective;
  state: PredictionState;
  state_note?: string;
  superseded_by?: string;
  /** Set while unresolved: negative before the match. */
  days_past_match_date?: number;
  issued_after_match_date?: boolean;
  population: SimulatorPopulation;
  claimed: {
    win_probability_team1: number;
    win_probability_source: WinProbabilitySource;
    predicted_winner_id: number;
    team1_range?: TrackRecordRange;
    team2_range?: TrackRecordRange;
    team1_players: number;
    team2_players: number;
  };
  happened?: {
    match_id: number;
    winner_opposition_id: number | null;
    outcome_by_runs?: number;
    outcome_by_wickets?: number;
    team1_total: number | null;
    team2_total: number | null;
    team1_batted_first: boolean | null;
  };
  score?: {
    team1_won: boolean;
    brier: number;
    team1_covered: boolean | null;
    team2_covered: boolean | null;
    eleven_overlap: { matched: number; of: number; team1_matched: number; team2_matched: number };
  };
  payload_error?: string;
};

/** GET /api/track-record: the record scored on read, misses included (P2-4). */
export type TrackRecord = {
  computed_at: string;
  today: string;
  total: number;
  states: Record<PredictionState, number>;
  win: {
    overall: TrackRecordWinScore;
    by_format: Record<string, TrackRecordWinScore>;
    reliability: TrackRecordReliabilityBin[];
    reliability_bins: number;
  };
  coverage: {
    rows: TrackRecordCoverageRow[];
    populations: Record<SimulatorPopulation, number>;
  };
  elevens: {
    n: number;
    mean_overlap: number | null;
    min_overlap: number | null;
    max_overlap: number | null;
    complete: number;
  };
  predictions: TrackRecordEntry[];
};

/** Whether the database holds matches the served run never saw (the B-2 state). */
export const RETRAIN_STATUSES = ['up_to_date', 'retrain_due', 'unknown'] as const;
export type RetrainStatus = (typeof RETRAIN_STATUSES)[number];

/** The code a live prediction past the limit is refused with (H-11), as ml-service raises it. */
export const RATINGS_STALE_CODE = 'RATINGS_STALE';

/**
 * How far back the served run's data boundary is, and whether that is far enough to refuse
 * with (H-11).
 *
 * The verdict is taken on `data_through` — the run's cutoff, the date its data was built
 * to — and never on `ratings_through`, the last match it folded in (SERVE-03). The second
 * is the cricket calendar's, not the pipeline's: between seasons it walks away from today
 * on its own, and the retrain the refusal names cannot move it.
 */
export type RatingsFreshness = {
  fresh: boolean;
  /** Days from `data_through` to today — the quantity compared against the limit. */
  data_age_days: number | null;
  max_age_days: number;
  /** The served run's cutoff: what the age counts from. */
  data_through: string | null;
  /** The last match the run folded in. Reported beside the verdict, never the verdict. */
  ratings_through: string | null;
  /** RATINGS_STALE when a live prediction would be refused; absent when it would not. */
  code?: string | null;
};

/** ml-service GET /xi/status, via the go-app proxy: run identity (H-16) and freshness (H-11). */
export type XiStatusResponse = {
  loaded: boolean;
  formats: string[];
  performance_formats?: string[];
  players: number;
  ratings_through: string | null;
  run_id?: string | null;
  manifest?: RunManifestSummary | null;
  /**
   * The last reload's refusal (D-6), naming the run it refused. With `loaded` false, why
   * nothing is serving; with `loaded` true, a reload named a run that could not be served
   * and `run_id` went on serving (B-13) -- the two fields describe two different runs.
   */
  error?: string | null;
  ratings?: RatingsFreshness | null;
  report?: Record<string, unknown> | null;
};

/** The short form of a run's manifest, as /xi/status carries it. */
export type RunManifestSummary = {
  run_id: string;
  created_at?: string;
  /** The training boundary the operator asked for; rows at or after it are the holdout. */
  cutoff?: string;
  /**
   * The last match the rating pass consumed — the date every prediction from this run
   * is "as of". Recorded by retrain and asserted against the state at load (P2-2), so
   * it is the same date the served-ratings stamp carries.
   */
  ratings_through?: string;
  dataset_sha?: string;
  git_sha?: string;
  /** The formats the run trained — not the formats it managed to score (B-3). */
  formats?: string[];
  /**
   * Why a format carries no headline metrics: it was trained and there was no holdout to
   * score it on, or it was not trained at all. Absent for a run that scored everything.
   */
  format_notes?: Record<string, string>;
  hyperparameters?: Record<string, unknown>;
  /**
   * The run's headline metrics per format, keyed by the metric key the service reports
   * them under — the same keys the glossary explains (L-1).
   */
  metrics?: Record<string, Record<string, number>>;
};

/** Model metadata from ml-service GET /model-metadata (via go-app proxy). One source of truth for Workbench UI. */
export type FoldStat = {
  mean: number;
  sd: number;
  n_folds: number;
  /** EVAL-11: how many gates have read the folds this summary averaged — the comparisons
   * a development number sits under. Absent on a bare holdout figure. */
  gates_consulted?: number;
};

/** EVAL-11: the holdout season's record, beside the locked window's numbers. */
export type HoldoutRecord = {
  n_matches: number;
  first_match: string | null;
  last_match: string | null;
  season_start: string;
  season_end: string;
  season_days: number;
  days_covered: number;
  season_complete: boolean;
  gates_consulted: number;
};

/** One walk-forward fold, or the locked window in the same shape. */
export type EvaluationFold = {
  cutoff: string;
  end: string;
  n_train: number;
  /** The locked window only: what the holdout holds and how much of its season has accrued. */
  holdout?: HoldoutRecord;
  n_eval: number;
  skipped_reason?: string;
  objective_auc?: number;
  objective_brier?: number;
  display_auc_mean?: number;
  display_brier_mean?: number;
  base_rate_brier?: number;
  swap_monotonicity?: { upgrades: number; violations: number; violation_share: number };
  display_swap_monotonicity?: { upgrades: number; violations: number; violation_share: number };
  specific_vs_typical?: {
    n: number;
    auc_specific_xi: number;
    auc_typical_xi: number;
    delta: number;
  };
  performance?: EvaluationPerformance;
  simulation?: EvaluationSimulation;
  note?: string;
  recalibrated_targets?: string[];
};

/** One forecast scored on one target and one population. Never pooled across targets (H-12). */
export type EvaluationTargetScore = {
  n?: number | FoldStat;
  mae?: number | FoldStat | null;
  within_match_spearman?: number | FoldStat | null;
  top3_hit_rate?: number | FoldStat | null;
  pinball?: number | FoldStat | null;
  /** Coverage and width of the 10-90 interval: width is the progress metric (H-22). */
  interval?: {
    coverage_80?: number | FoldStat;
    width_80?: number | FoldStat;
  } | null;
};

export type EvaluationPerformance = {
  targets?: Record<
    string,
    {
      headline?: boolean;
      model: EvaluationTargetScore;
      career_mean?: EvaluationTargetScore;
      career_quantiles?: EvaluationTargetScore;
    }
  >;
  skipped_reason?: string;
};

/** E2: the simulated match against the display model and against what actually happened. */
export type EvaluationSimulation = {
  n_matches?: number | FoldStat;
  skipped_reason?: string;
  win?: {
    brier?: {
      display: number | FoldStat;
      simulated: number | FoldStat;
      base_rate?: number | FoldStat;
    };
    delta_brier_simulated_minus_display?: number | FoldStat;
  };
  totals?: Record<string, EvaluationTotals>;
  /**
   * What this window's simulator was calibrated with. `shared_factor` is null where the
   * calibration window was too thin to fit one, which is the window's own half of B-12.
   */
  calibration?: { shared_factor?: unknown } | null;
  /** B-12: which folds simulated with a shared match factor, and their totals alone. */
  shared_factor_folds?: EvaluationSharedFactorFolds;
};

/**
 * B-12: a fold whose calibration window was too thin to fit the shared match factor ships
 * the un-widened simulator, so its intervals belong to a different model. The walk-forward
 * summary counts those folds, names their windows, and repeats the totals over the folds
 * that had a factor — the pooled figure and the calibrated-only one, both published.
 */
export type EvaluationSharedFactorFolds = {
  folds_scored: number;
  with_shared_factor: number;
  without_shared_factor: number;
  windows_without_shared_factor: string[];
  totals_with_shared_factor?: Record<string, EvaluationTotals> | null;
};

export type EvaluationTotals = {
  n?: number | FoldStat;
  coverage_80?: number | FoldStat;
  width_80?: number | FoldStat;
  dispersion_ratio?: number | FoldStat;
  median_mae?: number | FoldStat;
};

/** A sign-agreement figure with its denominator and sampling error. */
export type EvaluationAgreement = {
  pairs_scored: number;
  agreed?: number;
  agreement: number | null;
  standard_error?: number | null;
  ci95?: [number, number] | null;
  excluded_result_unchanged?: number;
  excluded_objective_indifferent?: number;
  skipped_reason?: string;
  note?: string;
};

/**
 * E5, lineup-only: one side's consecutive elevens 1–3 players apart, both scored in the
 * later fixture at its as-of, sign agreement with the result change. The bar is derived
 * from the objective's own claimed effect size (plan §8.8), not chosen.
 */
export type EvaluationE5 = {
  definition: string;
  why_not_as_played: string;
  pairs: { total: number; development: number; locked: number; unscored_previous_eleven?: number };
  walk_forward: {
    folds: Array<EvaluationAgreement & { cutoff: string; end: string; pairs: number }>;
    summary: { agreement: FoldStat | null };
  };
  development: EvaluationAgreement & {
    effect_size?: {
      n: number;
      median_abs: number | null;
      mean_abs?: number | null;
      p90_abs: number | null;
    };
    derived_bar?: {
      n_pairs: number;
      expected_if_exactly_right: number | null;
      simulated_mean?: number;
      simulated_sd?: number;
      bar: number | null;
      bar_quantile?: number;
      replicates?: number;
    };
    passes_derived_bar?: boolean | null;
  };
  locked: EvaluationAgreement;
  decision: EvaluationSelectionDecision;
};

/** Per format: what E5 said against which bar, and whether optimised selection is served. */
export type EvaluationSelectionDecision = {
  agreement: number | null;
  pairs_scored?: number;
  standard_error?: number | null;
  bar: number | null;
  expected_if_exactly_right?: number | null;
  passes_derived_bar: boolean | null;
  optimised_selection_served: boolean;
  reason: string;
};

/** The two anchors a metric's value is painted between, from the glossary's band. */
export type MetricScale = {
  bad: number;
  good: number;
};

/**
 * L-1: one reported metric key, explained by the service that computes it.
 *
 * The frontend holds no metric prose of its own: `name`, `explanation`, `band` and
 * `better` are rendered as they arrive, so rewording an explanation is a change to
 * `ml/xi/glossary.py` and never to a component.
 */
export type MetricGlossaryEntry = {
  key: string;
  /** Plain-language name, for a reader who has not lived inside the plan. */
  name: string;
  explanation: string;
  /** The reference this system measured, never a textbook value. */
  band: string;
  /** Which direction is progress, in words: "higher is better", and so on. */
  better: string;
  /**
   * The same judgement in one word, for a surface that colours a change rather than
   * printing a sentence. `nominal`, `exact` and `none` carry no better/worse verdict.
   */
  direction: 'higher' | 'lower' | 'nominal' | 'exact' | 'none';
  /**
   * The same reference `band` states in prose, as the two numbers a surface paints
   * between: fully red at `bad`, fully green at `good`, read through `direction`. Absent
   * where the metric has no defensible anchor of its own — those are shown uncoloured
   * rather than given a shade the harness never measured.
   */
  scale?: MetricScale | null;
};

export type MetricGlossary = {
  entries: Record<string, MetricGlossaryEntry>;
};

/** H-23: what a gate varies, what it holds fixed and what decides, beside its number. */
export type EvaluationGate = {
  id: string;
  name: string;
  varies: string;
  fixed: string;
  decides: string;
  report_path?: string | null;
  /**
   * The decides clause as the harness evaluates it on the number at `report_path` (EVAL-04);
   * null for a gate whose clause decides an action, informs, or is a script's to evaluate.
   */
  threshold?: string | null;
};

export type EvaluationFormatReport = {
  n_matches: number;
  walk_forward: {
    folds: EvaluationFold[];
    summary: {
      objective_auc?: FoldStat | null;
      objective_brier?: FoldStat | null;
      display_auc?: FoldStat | null;
      base_rate_brier?: FoldStat | null;
      swap_violation_share?: FoldStat | null;
      /** B-7: the same probe on the display surface. Reported, never a gate. */
      display_swap_violation_share?: FoldStat | null;
      specific_vs_typical_delta?: FoldStat | null;
      performance?: EvaluationPerformance | null;
      simulation?: EvaluationSimulation | null;
    };
  };
  locked: EvaluationFold;
  simulation_decision: {
    simulated_win_probability_within_tolerance: boolean;
    reason?: string;
    delta_brier_mean?: number;
    tolerance?: number;
    shared_factor?: boolean;
    chase_orientation?: string;
    served?: boolean;
  };
  e5_lineup_only?: EvaluationE5;
  selection_decision?: EvaluationSelectionDecision;
};

/** One window of X-4's market benchmark: the three arms, and the gaps between them. */
export type MarketBenchmarkWindow = {
  n: number;
  cutoff?: string;
  end?: string;
  market_auc?: number;
  market_brier?: number;
  display_auc_mean?: number;
  display_brier_mean?: number;
  display_toss_aware_auc?: number;
  display_toss_aware_brier?: number;
  market_minus_display_auc?: number;
  market_minus_display_brier?: number;
  market_minus_toss_aware_auc?: number;
  market_minus_display_auc_ci95?: number[] | null;
  market_minus_toss_aware_auc_ci95?: number[] | null;
  skipped_reason?: string;
  note?: string;
};

/** One format's market benchmark, coverage first: the numbers mean nothing without it. */
export type MarketBenchmarkFormat = {
  matches_in_windows: number;
  matches_joined: number;
  joined_share: number;
  folds: MarketBenchmarkWindow[];
  pooled: MarketBenchmarkWindow | null;
  locked: MarketBenchmarkWindow;
};

/**
 * X-4: the betting market scored beside the display model on the matches both cover. It
 * informs and decides nothing, and no model anywhere reads odds as a feature.
 */
export type MarketBenchmark = {
  available: boolean;
  source: {
    name: string;
    url: string;
    licence: string;
    cached_dir: string;
    priced_at: string;
    files?: string[];
  };
  devig: { method: string; note: string; market_overround: number | null };
  join: {
    quotes: number;
    quotes_joined: number;
    quotes_no_match: number;
    quotes_unknown_team: number;
    quotes_ambiguous: number;
    label_disagreements: number;
    quotes_joined_outside_scored_windows?: number;
    key: string;
  };
  formats: Record<string, MarketBenchmarkFormat>;
};

export type EvaluationReport = {
  generated_at: string;
  source: string;
  cutoffs: string[];
  locked_start: string;
  /** A-4: where the locked window's line is, and when it was last moved there. */
  locked_window?: {
    start: string;
    rotated_on: string;
    previous_start: string;
    reason: string;
    retired_into_folds: string[];
    /** EVAL-11: the season the holdout accrues from the line, and the day it completes. */
    season_days?: number;
    season_end?: string;
  };
  n_rows: number;
  n_player_rows: number;
  data_quality?: Record<string, unknown>;
  leak_canary?: {
    best_single_column?: Record<string, { column: string; auc: number }>;
    test_control_suspects?: unknown[];
  };
  formats: Record<string, EvaluationFormatReport>;
  /** X-4: the market benchmark, absent from reports written before it existed. */
  market_benchmark?: MarketBenchmark;
  serving_parity: {
    passed: boolean;
    mismatches?: unknown[];
    [key: string]: unknown;
  };
  /** The gate registry the harness checked the report against (H-23). */
  gates?: {
    registry: Record<string, EvaluationGate>;
    passed?: boolean;
    problems?: string[];
  };
  /** The metric glossary the harness embedded, and whether it explained every metric (L-1). */
  glossary?: MetricGlossary & {
    passed?: boolean;
    problems?: string[];
  };
};

// --- Ops Status (go-app API) DTO ---
export type TableStat = {
  table_name: string;
  row_count: number;
  last_record?: string;
};

export type OpsStatusDTO = {
  timestamp: string;
  services?: {
    api_health?: boolean;
    api_readiness?: boolean;
    ml_health?: boolean;
  };
  db?: {
    connected?: boolean;
    counts?: Record<string, number>;
    migration?: {
      status?: string;
      current?: number;
      expected?: number;
    };
    last_match_import_at?: string;
    table_stats?: TableStat[];
    [key: string]: unknown;
  };
  /** The runs on disk, which is current and which is loaded (H-16), plus H-11's verdict. */
  artifacts?: {
    root?: string;
    reachable?: boolean;
    current_run?: string | null;
    loaded_run?: string | null;
    ratings_through?: string | null;
    ratings?: {
      fresh?: boolean;
      data_age_days?: number | null;
      max_age_days?: number;
      data_through?: string | null;
    } | null;
    error?: string | null;
    runs?: Array<{
      run_id?: string;
      created_at?: string;
      cutoff?: string;
      /** The run's data date, off its manifest — answered for every run on disk (P2-2). */
      ratings_through?: string | null;
      git_sha?: string;
      dataset_sha?: string;
      formats?: string[];
      has_manifest?: boolean;
      /** Why this run cannot be loaded, when it cannot; null on a loadable run (§8.7). */
      refused?: string | null;
      current?: boolean;
      loaded?: boolean;
    }>;
  };
  /** Pipeline step running state from backend */
  pipeline?: {
    steps?: Record<string, { running?: boolean }>;
  };
  [key: string]: unknown;
};

/** Response from POST /ops/pipeline/run/:step (202 started, 501 run from root, 4xx/5xx error) */
export type PipelineRunResponse = {
  status?: string;
  step?: string;
  error?: string;
  command?: string;
  /** Machine-readable failure reason, e.g. UNIFIED_MODEL_REMOVED. */
  code?: string;
  message?: string;
  /** The next action to take. Written to be shown, not swallowed. */
  hint?: string;
  /** The named plan the step started, when it is one — `import` runs a plan. */
  plan?: string;
  /** The plan's steps, in the order they will run. */
  steps?: string[];
  /** The archive URL the plan resolved, redacted. */
  source_url?: string;
  /**
   * Which of the plan's steps will not run, keyed by step id, and why.
   *
   * Import is a fetch -> extract -> import plan that skips acquisition when the
   * dataset directory already holds the configured archive. Showing the backend's
   * own reason is what keeps a skipped download from reading as a completed one.
   */
  skipped?: Record<string, string>;
};

/**
 * Response from POST /ops/data/fetch and /ops/data/extract.
 *
 * 202 carries the started job; 400 and 409 carry `error` plus the context needed to
 * act on it — `allowed_hosts` for a refused source, `archives` for "nothing staged".
 */
export type OpsDataStartResponse = {
  status?: string;
  step?: string;
  feed?: string;
  url?: string;
  filename?: string;
  archive?: string;
  dest_dir?: string;
  error?: string;
  allowed_hosts?: string[];
  staging_dir?: string;
  archives?: StagedArchive[];
};

/** Where one step of a run plan has got to. */
export type RunPlanStepStatus =
  'PENDING' | 'RUNNING' | 'COMPLETED' | 'FAILED' | 'CANCELLED' | 'SKIPPED';

export type RunPlanStep = {
  step_id: string;
  label: string;
  status: RunPlanStepStatus;
  started_at?: string;
  finished_at?: string;
  /** Actionable where ml-service supplied a code and a hint. */
  error?: string;
  /**
   * Why a step was skipped, in the backend's words.
   *
   * A skipped step that renders as "done" is a claim the run cannot back up. The
   * reason is what makes "skipped" checkable — "the dataset directory already holds
   * all_json.zip from this source (21,253 match files)" is something an operator can
   * go and verify.
   */
  note?: string;
  migration_id?: number;
};

/**
 * Payload of GET /ops/pipeline/plan — the latest plan, running or not.
 *
 * "Latest" rather than "current" is deliberate: the state lives in the database, so a
 * plan shows up after a page reload, from another tab, or the morning after the
 * browser that started it was closed.
 */
export type RunPlanState = {
  id?: number;
  running: boolean;
  plan?: string;
  steps?: RunPlanStep[];
  started_at?: string;
  finished_at?: string;
  /**
   * How the run ended. Absent while it is still going, and absent for a run that
   * ended before the backend recorded this.
   *
   * Reported rather than inferred from the step list: a plan whose last step reads
   * CANCELLED could have been stopped by the operator or could have had its final
   * step time out, and "was this stopped, or did it break?" is the first question
   * asked of a pipeline that did not finish.
   */
  outcome?: 'COMPLETED' | 'FAILED' | 'CANCELLED';
  /** Where a resume would start. Absent while the plan is running. */
  resume_from?: string;
  /** The plan names the backend accepts. */
  plans: string[];
};

/** Response from POST /ops/pipeline/run-plan. */
export type RunPlanStartResponse = {
  status?: string;
  plan?: string;
  steps?: string[];
  resume?: boolean;
  error?: string;
  plans?: string[];
};

/** A named Cricsheet archive the server will fetch, from GET /ops/data/feeds. */
export type DataFeed = {
  id: string;
  label: string;
  url: string;
  description: string;
};

/** Payload of GET /ops/data/feeds. */
export type DataFeedsResponse = {
  feeds: DataFeed[];
  /**
   * Hosts the server will fetch from. Stated in the UI so the rule is visible before
   * a URL is typed rather than discovered by being refused.
   */
  allowed_hosts: string[];
  staging_dir: string;
};

/** A downloaded archive waiting to be extracted, from GET /ops/data/staged. */
export type StagedArchive = {
  filename: string;
  bytes: number;
  modified: string;
  /** Provenance the fetch recorded. Absent for an archive placed there by hand. */
  sha256?: string;
  source_url?: string;
  feed_id?: string;
  fetched_at?: string;
  last_modified?: string;
};

/** The manifest an extraction leaves in the dataset directory. */
export type DatasetManifest = {
  archive_path?: string;
  archive_sha256?: string;
  source_url?: string;
  feed_id?: string;
  dest_dir?: string;
  entries?: number;
  match_files?: number;
  bytes?: number;
  replaced_into?: string;
  extracted_at?: string;
};

/** Payload of GET /ops/data/staged. */
export type StagedResponse = {
  staging_dir: string;
  archives: StagedArchive[] | null;
  dataset_dir: string;
  /** Absent when the dataset directory has no manifest — provenance genuinely unknown. */
  live?: DatasetManifest;
};

/**
 * One acquired dataset, from GET /ops/data/datasets.
 *
 * Optional fields are genuinely unknown rather than zero: an archive placed in
 * staging by hand has no feed or source URL, and one that has been downloaded but not
 * extracted has no entry count. Render absence as "unknown", never as 0.
 */
export type DatasetRegistryEntry = {
  id: number;
  /** SHA-256 of the archive. This, not the filename, identifies a dataset. */
  sha256: string;
  feed?: string;
  source_url?: string;
  filename: string;
  bytes: number;
  etag?: string;
  last_modified?: string;
  fetched_at?: string;
  extracted_at?: string;
  entry_count?: number;
  /** The subset of entries the importer will read. */
  match_files?: number;
  extracted_bytes?: number;
  dest_dir?: string;
  created_at: string;
  updated_at: string;
  /** True for the dataset currently in the data directory, derived from its manifest. */
  live: boolean;
};

/** Payload of GET /ops/data/datasets. */
export type DatasetRegistryResponse = {
  datasets: DatasetRegistryEntry[];
  dataset_dir: string;
  /**
   * Digest of the dataset in the data directory. It can be set while no entry is
   * marked live: that means the directory holds a dataset the registry has never
   * seen, which is a state to show rather than hide.
   */
  live_sha256: string;
};

/**
 * Biography coverage for one format and gender, weighted by appearances (X-1a).
 *
 * Every count is a count of fielded player-sides, not of players: a biography for
 * someone who played once in 2004 is worth less than one for someone in every eleven
 * this season, and a player-weighted figure counts them the same.
 */
export type BiographyCoverageRow = {
  format: string;
  gender: string;
  appearances: number;
  /** Appearances whose player has a biography row at all — the pass looked him up. */
  attempted: number;
  /** Appearances whose player was found on Wikidata. */
  matched: number;
  birth_date: number;
  batting_hand: number;
  /** Only styles the controlled vocabulary could place; a stated but unplaceable style is not coverage. */
  bowling_style: number;
  career_end: number;
  death: number;
  players: number;
  matched_players: number;
};

/** A player the biography pass found nothing for, with what fixing him would be worth. */
export type BiographyUnmatchedPlayer = {
  player_id: number;
  name: string;
  cricsheet_id: string;
  cricinfo_id?: string;
  appearances: number;
  /** False when there is no biography row at all: the pass has not run for him. */
  attempted: boolean;
};

/** GET /ops/data/biography-coverage: the state of the acquired player biographies. */
export type BiographyCoverageResponse = {
  generated_at: string;
  rows: BiographyCoverageRow[];
  total: BiographyCoverageRow;
  unmatched: BiographyUnmatchedPlayer[];
  /** Newest fetched_at in the table; absent when the acquisition has never run. */
  last_fetched_at?: string;
  source_license: string;
};

/**
 * Resource a step contends for. Steps in one lane run one at a time; the lanes
 * overlap, so a dataset download and a training run can be in flight together.
 * Generated backend-side from the step registry (contracts/ops-console.contract.json).
 */
export type PipelineLane = 'compute' | 'data';

/** Live progress for one running pipeline step. */
export type PipelineStepProgress = {
  step_id?: string;
  step_label?: string;
  /** Resource the step contends for: 'compute' (db and artifacts) or 'data' (acquisition). */
  lane?: PipelineLane;
  /** Human-readable description of what is happening */
  detail?: string;
  /** Current parameters (e.g. model, format, cutoff) for display */
  params?: Record<string, unknown>;
  started_at?: string;
  elapsed_sec?: number;
  estimated_remaining_sec?: number;
  /** Live download progress for a dataset fetch. Absent until the first sample. */
  fetch?: {
    downloaded_bytes?: number;
    /** Declared Content-Length, absent when the server sent none. */
    total_bytes?: number;
    bytes_per_sec?: number;
    eta_sec?: number;
  };
  /** Live extraction progress for a dataset extract. Absent until the first sample. */
  extract?: {
    entries?: number;
    entries_total?: number;
    bytes?: number;
    eta_sec?: number;
  };
  /**
   * Milestones published by a trainer: rows loaded, low-variance columns dropped,
   * CV folds, artifacts written. Absent until the step publishes its first event.
   *
   * The envelope is versioned (`v`); step-specific fields sit beside it at the top
   * level, so this is deliberately open rather than a closed shape.
   */
  training?: {
    v?: number;
    run_id?: string;
    step?: string;
    phase?: string;
    current?: number;
    total?: number;
    metrics?: Record<string, number>;
    message?: string;
    ts?: string;
    format?: string;
    dropped_columns?: string[];
    dropped_columns_truncated?: number;
    artifacts?: { path: string; bytes: number }[];
    [key: string]: unknown;
  };
  /**
   * True when ml-service could not be asked, as distinct from it answering "nothing
   * published yet". Both render as an empty panel otherwise, but one is a run about
   * to report and the other is a broken link the operator can act on.
   */
  progress_unavailable?: boolean;
};

/**
 * Payload of the SSE "progress" event from GET /ops/pipeline/stream.
 *
 * `steps` carries every in-flight step, most recently started first. It is a list
 * because more than one step can run at once: acquisition has its own lane, and the
 * run-plan executor will drive several. Never assume `steps[0]` is the only one.
 */
export type PipelineProgressPayload = {
  running: boolean;
  steps: PipelineStepProgress[];
};

/** The `dataset` section of /ops/status: what is in the server's Cricsheet directory. */
export type DatasetStatus = {
  path: string;
  exists: boolean;
  readable: boolean;
  /** Files Import will read: *.json directly in the directory, not recursive. */
  match_files: number;
  bytes: number;
  newest_file?: string;
  newest_modified?: string;
  empty: boolean;
  error?: string;
  /** Name of the environment variable that overrides the directory. */
  env_var?: string;
};

/**
 * What a finished run is remembered by, from `data_migrations.metadata`.
 *
 * `provenance` is what makes "which data produced this model?" a lookup rather than
 * an archaeology exercise. Its fields are individually optional because a dataset
 * placed by hand has a digest but no feed or URL, and one placed before the registry
 * existed has none of them — absent means genuinely unknown, never zero.
 */
export type RunMetadata = {
  step?: string;
  cutoff?: string;
  summary?: {
    v?: number;
    step?: string;
    run_id?: string;
    saved?: number;
    formats_completed?: number;
    formats_total?: number;
    finished_at?: string;
    formats?: {
      format: string;
      rows?: number;
      features?: number;
      targets?: number;
      completed?: boolean;
      metrics?: Record<string, number>;
      artifacts?: { path: string; bytes: number }[];
    }[];
    /** Low-variance columns removed before fitting, keyed by format. */
    dropped_columns?: Record<string, string[]>;
    [key: string]: unknown;
  };
  provenance?: {
    dataset_sha256?: string;
    dataset_source_url?: string;
    dataset_feed?: string;
    dataset_extracted_at?: string;
    dataset_match_files?: number;
  };
  [key: string]: unknown;
};

export type Migration = {
  id: number;
  command: string;
  args: unknown;
  started_at: string;
  completed_at?: string;
  status: 'IN_PROGRESS' | 'COMPLETED' | 'FAILED' | 'CANCELLED';
  /**
   * Untyped for rows written before O-4 and by steps go-app runs itself, which record
   * their own shapes. `RunMetadata` describes what a training step now writes.
   */
  metadata?: RunMetadata | unknown;
  error_message?: string;
};

export type Suggestion = {
  title: string;
  description: string;
  command: string;
  priority: string;
};

export type FormatHierarchyNode = {
  code: string;
  name: string;
  children?: FormatHierarchyNode[];
};

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  limit: number;
}
