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
};

/**
 * How the XIs were chosen. `optimised` is false where the win objective does not rank
 * (H-17: TEST), and every surface showing such an XI has to say so.
 */
export type PredictSelectionSummary = {
  objective: 'win' | 'ratings';
  optimised: boolean;
  note?: string;
};

/**
 * The headline win probability and which model produced it. The other model's answer is
 * reported beside it, never blended with it.
 */
export type PredictWinProbability = {
  team1: number;
  source: 'display' | 'simulator';
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

/** The simulated match. Absent for a format with no innings length. */
export type PredictScorecard = {
  samples: number;
  toss_marginalised: boolean;
  innings1: PredictInningsTotal;
  innings2: PredictInningsTotal;
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

/** Why the retirement ledger left a candidate out of the pool. */
export const POOL_EXCLUSION_REASONS = ['user_flagged', 'retired'] as const;
export type PoolExclusionReason = (typeof POOL_EXCLUSION_REASONS)[number];

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

export type PredictTeamSelectionResponse = {
  /** The sides that were actually scored, echoed back whether or not the request was clear. */
  team1_side: TeamSideOption;
  team2_side: TeamSideOption;
  team1: PredictTeamSelectedPlayer[];
  team2: PredictTeamSelectedPlayer[];
  selection: PredictSelectionSummary;
  win_probability: PredictWinProbability;
  scorecard?: PredictScorecard;
  /** Which candidates each XI was chosen out of, and who the ledger excluded (D-12). */
  team1_pool: PoolSummary;
  team2_pool: PoolSummary;
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

/** How old the loaded rating state is, and whether that is old enough to refuse with. */
export type RatingsFreshness = {
  fresh: boolean;
  age_days: number | null;
  max_age_days: number;
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
  /** Why nothing is loaded, when a run on disk was refused (D-6). */
  error?: string | null;
  ratings?: RatingsFreshness | null;
  report?: Record<string, unknown> | null;
};

/** The short form of a run's manifest, as /xi/status carries it. */
export type RunManifestSummary = {
  run_id: string;
  created_at?: string;
  cutoff?: string;
  dataset_sha?: string;
  git_sha?: string;
  formats?: string[];
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
};

/** One walk-forward fold, or the locked window in the same shape. */
export type EvaluationFold = {
  cutoff: string;
  end: string;
  n_train: number;
  n_eval: number;
  skipped_reason?: string;
  objective_auc?: number;
  objective_brier?: number;
  display_auc_mean?: number;
  display_brier_mean?: number;
  base_rate_brier?: number;
  swap_monotonicity?: { upgrades: number; violations: number; violation_share: number };
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
  };
  seeds: number[];
  n_rows: number;
  n_player_rows: number;
  data_quality?: Record<string, unknown>;
  leak_canary?: {
    best_single_column?: Record<string, { column: string; auc: number }>;
    test_control_suspects?: unknown[];
  };
  formats: Record<string, EvaluationFormatReport>;
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
    ratings?: { fresh?: boolean; age_days?: number | null; max_age_days?: number } | null;
    error?: string | null;
    runs?: Array<{
      run_id?: string;
      created_at?: string;
      cutoff?: string;
      git_sha?: string;
      dataset_sha?: string;
      formats?: string[];
      has_manifest?: boolean;
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
