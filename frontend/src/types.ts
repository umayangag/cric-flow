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
            }
          | undefined
        >
      | undefined;
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
  metadata?: unknown;
  error_message?: string;
};

export type Suggestion = {
  title: string;
  description: string;
  command: string;
  priority: string;
};

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  limit: number;
}
