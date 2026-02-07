export type PlayerPrediction = {
  player_name: string;
  runs_scored: number;
  balls_faced: number;
  fours_scored: number;
  sixes_scored: number;
  batting_position: number;
  strike_rate: number;
  runs_conceded: number;
  deliveries: number;
  wickets_taken: number;
  econ: number;
  winning_probability?: number | null;
};

export type TeamWinResponse = {
  players: PlayerPrediction[];
  team_win_probability: number;
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

export type CsvRow = PlayerPrediction & {
  match_id: string | number;
  date: string; // YYYY-MM-DD
  season?: number; // optional, can derive from date
  team_name: string;
  actual_win: number; // 0|1 for team outcome
};

// --- DB-backed API DTOs ---
export type SeasonsNextResponse = {
  next_season: number | null;
};

export type MatchListItem = {
  match_id: number | string;
  date: string; // YYYY-MM-DD
  format?: string;
  teams: [string, string];
};

export type SquadDTO = {
  team_name: string;
  actual_win: 0 | 1;
  players: PlayerPrediction[];
};

export type MatchSquadsResponse = {
  match_id: number | string;
  date: string; // YYYY-MM-DD
  teams: [string, string];
  squads: [SquadDTO, SquadDTO]; // exactly two
};

// --- Backtest API DTOs ---
export type BacktestCandidate = {
  match_id: number;
  stable_id: string;
  date: string; // RFC3339
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
  match: { match_id: number; date: string };
  players: BacktestEvaluatePlayerRow[];
  metrics: Record<string, number>; // e.g., { player_runs_mae: 3.66 }
  match_aggregates?: {
    predicted: Record<string, number | string>;
    actual: Record<string, number | string>;
    errors: Record<string, number>;
  };
};

// --- Ops Status (go-app API) DTO ---
export type OpsStatusDTO = {
  timestamp: string;
  services?: {
    api_health?: boolean;
    api_readiness?: boolean;
    ml_health?: boolean;
  };
  db?: unknown;
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
  suggestions?: Array<{ reason: string; commands: string[] }>;
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
