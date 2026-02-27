// --- Upcoming match prediction ---
export type PredictTeamSelectedPlayer = {
  player_id: number;
  player_name: string;
  runs: number;
  wickets: number;
  economy: number;
  catches: number;
  run_outs: number;
};

export type PredictTeamSelectionResponse = {
  team1: PredictTeamSelectedPlayer[];
  team2: PredictTeamSelectedPlayer[];
};

export type HealthResponse = {
  status: string;
  loaded_batting_formats: string[];
  loaded_bowling_formats: string[];
  legacy_batting_available: boolean;
  legacy_bowling_available: boolean;
  models_dir: string;
  artifacts: {
    batting: { file: string; size_bytes?: number; modified?: number }[];
    bowling: { file: string; size_bytes?: number; modified?: number }[];
  };
  metadata: {
    batting: string[];
    bowling: string[];
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
export type ModelMetadataResponse = Record<string, ModelMetadataEntry>;

/** MLQA audit from auto_tune MLQA Agent. */
export type MLQAAudit = {
  audit_status: 'PASS' | 'FAIL' | 'WARNING';
  key_findings: string[];
  bias_report: string;
  final_verdict: string;
  checks?: {
    overfitting?: { delta: number; flagged: boolean };
    stability?: { cv_std: number; flagged: boolean };
  };
};

/** ML model stats from ml-service GET /model-stats (via go-app proxy). Used by ML Model Stats tab. */
export type MLModelStat = {
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
  accuracy_display?: string;
  /** MLQA audit (overfitting, stability, bias, sensitivity, complexity). */
  mlqa_audit?: MLQAAudit;
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
  message?: string;
};

/** Payload of SSE "progress" event from GET /ops/pipeline/stream */
export type PipelineProgressPayload = {
  running: boolean;
  step_id?: string;
  step_label?: string;
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
  };
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
  /** When true, use the unified (legacy) model for predictions instead of the format-specific model. */
  use_unified_model?: boolean;
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
