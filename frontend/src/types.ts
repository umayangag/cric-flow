// --- Upcoming match prediction ---
export type PredictTeamSelectedPlayer = {
  player_id: number;
  player_name: string;
  runs: number;
  balls?: number;
  fours?: number;
  sixes?: number;
  wickets: number;
  economy: number;
  catches: number;
  run_outs: number;
};

/** Scorecard summary: innings totals and winner; when win model is used, totals are reconciled to win probability. */
export type PredictScorecardSummary = {
  innings1_total: number;
  innings2_total: number;
  predicted_winner: string;
  team1_win_probability?: number;
  extras_innings1?: number;
  extras_innings2?: number;
};

/** Monte Carlo simulation result (when simulate=true on team-selection). */
export type PredictSimulationResult = {
  win_probability_team1: number;
  win_probability_team2: number;
  draw_probability: number;
  innings1_total_mean: number;
  innings1_total_std: number;
  innings1_total_p10: number;
  innings1_total_p50: number;
  innings1_total_p90: number;
  innings2_total_mean: number;
  innings2_total_std: number;
  innings2_total_p10: number;
  innings2_total_p50: number;
  innings2_total_p90: number;
  num_matchups: number;
  num_samples: number;
};

export type PredictTeamSelectionResponse = {
  team1: PredictTeamSelectedPlayer[];
  team2: PredictTeamSelectedPlayer[];
  /** Baseline scorecard summary from per-innings models. */
  scorecard_summary?: PredictScorecardSummary;
  /** Reconciled scorecard summary aligned with win-model probabilities (when available). */
  scorecard_summary_reconciled?: PredictScorecardSummary;
  /** Present when simulate=true; win probs and innings distributions from Monte Carlo. */
  simulation?: PredictSimulationResult;
};

export type HealthResponse = {
  status: string;
  loaded_batting_formats: string[];
  loaded_bowling_formats: string[];
  loaded_fielding_formats?: string[];
  loaded_extras_formats?: string[];
  loaded_win_formats?: string[];
  models_dir: string;
  artifacts: {
    batting: { file: string; size_bytes?: number; modified?: number }[];
    bowling: { file: string; size_bytes?: number; modified?: number }[];
    fielding?: { file: string; size_bytes?: number; modified?: number }[];
    extras?: { file: string; size_bytes?: number; modified?: number }[];
    win?: { file: string; size_bytes?: number; modified?: number }[];
  };
  metadata: {
    batting: string[];
    bowling: string[];
    fielding?: string[];
  };
};

/** Model metadata from ml-service GET /model-metadata (via go-app proxy). One source of truth for Workbench UI. */
export type ModelMetadataEntry = {
  features: string[];
  outputs: string[];
  level: 'player' | 'match' | 'meta';
  hasScaler?: boolean;
  artifactsPattern: { perFormat: string; legacy: string };
  note?: string;
};

/** Model mode (legacy vs per_format) from model-metadata; used for Prediction model selector labels/descriptions. */
export type ModelModeEntry = {
  name: string;
  description?: string;
  available?: boolean;
  deprecated?: boolean;
};

/** Full API response: model_modes + one entry per model kind (batting, bowling, etc.). */
export type ModelMetadataApiResponse = {
  model_modes?: ModelModeEntry[];
} & Record<string, ModelMetadataEntry | ModelModeEntry[] | undefined>;

/** Map of model kind -> entry only (no model_modes). Used where we iterate model entries. */
export type ModelMetadataResponse = Record<string, ModelMetadataEntry>;

/** MLQA audit from auto_tune MLQA Agent. */
export type MLQAAudit = {
  audit_status: 'PASS' | 'FAIL' | 'WARNING';
  key_findings: string[];
  bias_report: string;
  final_verdict: string;
  checks?: {
    overfitting?: { delta: number; relative_delta?: number; threshold?: number; flagged: boolean };
    stability?: {
      cv_std: number;
      relative_cv_std?: number;
      threshold?: number;
      flagged: boolean;
      cv_fold_scores?: number[];
    };
  };
};

/** ML model stats from ml-service GET /model-stats (via go-app proxy). Used by ML Model Stats tab. */
export type MLModelStat = {
  /** Machine-readable kind (e.g. "batting_share", "innings"). */
  model_kind?: string;
  model_name: string;
  match_format: string;
  size_bytes?: number;
  modified?: string;
  tuned?: boolean;
  best_cv_score?: number;
  scoring?: string;
  algorithm?: string;
  tuned_parameters?: Record<string, unknown>;
  cv_splits?: number;
  validation_method?: string;
  n_samples?: number;
  n_features?: number;
  metrics?: Record<string, unknown>;
  /** Feature importance from auto-tuning (tree-based models only). */
  feature_importance?: Record<string, number>;
  accuracy_display?: string;
  /** MLQA audit (overfitting, stability, bias, sensitivity, complexity). */
  mlqa_audit?: MLQAAudit;
  /** Algorithms used in last auto-tune run for this model+format (for default selection in UI). */
  algorithms_requested?: string[];
  /** Training start time (from linked data_migration) — when auto_tune run started. */
  trained_at?: string;
  /** Training completion time (from linked data_migration). */
  completed_at?: string;
  /** Training duration in seconds (from data_migration.completed_at - started_at). */
  duration_seconds?: number;
};
export type ModelStatsResponse = {
  models_dir: string;
  models: MLModelStat[];
  hierarchy?: FormatHierarchyNode[];
};

// --- Backtest API DTOs ---
export type BacktestCandidate = {
  match_id: number;
  stable_id: string;
  match_date: string; // RFC3339
  venue: string;
  season: string;
  format: string;
  team1: string;
  team2: string;
  winner_team_code: string;
};

export type BacktestSelectResponse = {
  filters: Record<string, unknown> & {
    format: string;
    team1: string;
    team2: string;
  };
  candidates: BacktestCandidate[];
};

export type BacktestEvaluatePlayerRow = {
  player_id: number;
  // Notes: backend may include optional bowling and fielding keys when available:
  // - Bowling: wickets, economy
  // - Fielding: catches, run_outs
  // Keys are additive and backward compatible.
  predicted: Record<string, number>; // e.g., { runs: 25, wickets: 1, economy: 7.5, catches: 2, run_outs: 1 }
  actual: Record<string, number>; // e.g., { runs: 30, wickets: 2, economy: 7.2, catches: 1, run_outs: 0 }
  errors: Record<string, number>; // e.g., { runs_mae: 5, wickets_mae: 1, economy_mae: 0.3, catches_mae: 1, run_outs_mae: 1 }
};

export type BacktestEvaluateResponse = {
  filters: Record<string, unknown> & {
    format: string;
    team1: string;
    team2: string;
    match_id: number;
  };
  match: { match_id: number; match_date: string };
  players: BacktestEvaluatePlayerRow[];
  metrics: Record<string, number>; // e.g., { player_runs_mae: 3.66 }
  match_aggregates?: {
    predicted: Record<string, number | string>;
    actual: Record<string, number | string>;
    errors: Record<string, number>;
  };
  /** Predicted scorecard from ML (data strictly before match date). Same shape as match scorecard. */
  predicted_scorecard?: MatchScorecardResponse | null;
};

// --- Evaluate job (persisted across refresh) ---
export type EvaluateJobStep = { step: string; message: string };

export type EvaluateStatusResponse = {
  job_id: string;
  match_id: string;
  format: string;
  team1: string;
  team2: string;
  /** True when the latest model was used (vs strict temporal cutoff). */
  use_latest_model?: boolean;
  status: 'running' | 'done' | 'error';
  steps?: EvaluateJobStep[];
  result?: BacktestEvaluateResponse | null;
  error?: string;
  created_at: string;
  updated_at: string;
};

// --- Match scorecard (for Evaluate DB tab) ---
export type ScorecardBatting = {
  player_name: string;
  runs: number | null;
  balls: number | null;
  fours: number | null;
  sixes: number | null;
  strike_rate: number | null;
  how_out: string | null;
};

export type ScorecardBowling = {
  player_name: string;
  overs: number | null;
  maidens: number | null;
  runs: number | null;
  wickets: number | null;
  economy: number | null;
  wides: number | null;
  no_balls: number | null;
  balls: number | null;
};

export type ScorecardInning = {
  inning_number: number;
  batting_team_name: string;
  bowling_team_name: string;
  runs_scored: number;
  wickets_lost: number;
  extras: number;
  target_runs?: number | null;
  batting: ScorecardBatting[];
  bowling: ScorecardBowling[];
};

export type MatchScorecardResponse = {
  match_id: number;
  match_date: string;
  venue: string;
  innings: ScorecardInning[];
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
  precompute?: {
    formats?: Record<string, { status?: 'ok' | 'stale' | 'missing' | string } | undefined>;
  };
  exports?: {
    formats?: Record<string, { files?: Array<{ name?: string; exists?: boolean }> } | undefined>;
  };
  artifacts?: {
    formats?:
      | Record<
          string,
          | {
              batting?: { exists?: boolean; loaded?: boolean };
              bowling?: { exists?: boolean; loaded?: boolean };
              fielding?: { exists?: boolean; loaded?: boolean };
              extras?: { exists?: boolean; loaded?: boolean };
              win?: { exists?: boolean; loaded?: boolean };
            }
          | undefined
        >
      | undefined;
    unified?: {
      batting?: { exists?: boolean; loaded?: boolean };
      bowling?: { exists?: boolean; loaded?: boolean };
      fielding?: { exists?: boolean; loaded?: boolean };
      extras?: { exists?: boolean; loaded?: boolean };
      win?: { exists?: boolean; loaded?: boolean };
    };
  };
  /** Pipeline step running state from backend */
  pipeline?: {
    steps?: Record<string, { running?: boolean }>;
  };
  [key: string]: unknown;
};

/** Response from POST /ops/pipeline/run/:step (202 started, 501 run from root, 200 requires_confirmation, 4xx/5xx error) */
export type PipelineRunResponse = {
  status?: string;
  step?: string;
  error?: string;
  command?: string;
  /** When true, no auto-tuned params in DB; UI should prompt before training with default config */
  requires_confirmation?: boolean;
  /** Machine-readable failure reason, e.g. UNIFIED_MODEL_REMOVED. */
  code?: string;
  message?: string;
  /** The next action to take. Written to be shown, not swallowed. */
  hint?: string;
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
  precompute?: {
    formats?: string[];
    current_format?: string;
    phase?: string;
    current_index?: number;
    formats_total?: number;
  };
  estimated_remaining_sec?: number;
  /** Live auto-tune progress: phase, algorithm, hyperparams, trial, trials_total, best_score, etc. */
  auto_tune?: {
    phase?: string;
    model_kind?: string;
    format_suffix?: string;
    algorithm?: string;
    hyperparams?: Record<string, unknown>;
    trial?: number;
    trials_total?: number;
    best_score?: number;
    best_algorithm?: string;
    message?: string;
    algorithms_screened?: string[];
    algorithms_requested?: string[];
    activity?: string;
  };
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

export type Migration = {
  id: number;
  command: string;
  args: unknown;
  started_at: string;
  completed_at?: string;
  status: 'IN_PROGRESS' | 'COMPLETED' | 'FAILED' | 'CANCELLED';
  metadata?: unknown;
  error_message?: string;
};

// --- Ops: Auto-tune migration details ---
export type AutoTuneRunDetailsEntry = {
  id: number;
  model: string;
  format: string;
  created_at: string;
  params?: Record<string, unknown>;
  metrics?: Record<string, unknown>;
};

export type AutoTuneRunDetailsResponse = {
  migration_id: number;
  runs: AutoTuneRunDetailsEntry[];
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

// --- Workbench: accuracy trend (go-app /api/backtest/accuracy-trend) ---
export type AccuracyTrendItem = {
  match_id: number;
  match_date: string;
  format: string;
  team1: string;
  team2: string;
  metrics: Record<string, number>;
};

export type AccuracyTrendResponse = {
  filters: Record<string, unknown>;
  count: number;
  results: AccuracyTrendItem[];
  summary: Record<string, number>;
  progressive: Array<Record<string, number>>;
};

export type AccuracyTrendFilters = {
  format?: string;
  start_date?: string;
  end_date?: string;
  team1?: string;
  team2?: string;
  order?: 'asc' | 'desc';
  limit?: number;
  cache?: 'off' | 'read' | 'readwrite';
  metrics?: string;
};

// --- Workbench: walk-forward registry (from walk_forward_registry.json) ---
export type WalkForwardWindowEntry = {
  run_id?: string;
  model_type: string;
  format: string;
  cutoff_trained_before: string;
  window_x: number;
  window_start_date?: string;
  window_end_date?: string;
  training_params?: Record<string, unknown>;
  auto_tune_used?: boolean;
  metrics: Record<string, number>;
  n_training_samples?: number;
  n_holdout_samples?: number;
  window_index?: number;
  created_at?: string;
  artifact_paths?: { scaler?: string; model?: string } | null;
  error?: string;
};

export type WalkForwardRegistry = {
  run_id: string;
  windows: WalkForwardWindowEntry[];
  config?: { initial_cutoff?: string; window_x?: number; format?: string };
};
