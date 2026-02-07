export type BacktestCandidate = {
  match_id: number;
  stable_id: string;
  date: string;
  venue: string;
  season: string;
  format: string;
  team1: string;
  team2: string;
  winner_team_code: string;
};

export type BacktestSelectResponse = {
  filters: Record<string, unknown>;
  candidates: BacktestCandidate[];
};

export type BacktestPlayerResult = {
  player_id: number;
  predicted: Record<string, number>;
  actual: Record<string, number>;
  errors: Record<string, number>;
};

export type BacktestEvaluateResponse = {
  filters: Record<string, unknown> & { model_version?: string };
  match: { match_id: number; date: string };
  match_aggregates?: {
    predicted: Partial<{ runs: number; wickets: number; extras: number; winner_team_code: string }>;
    actual: Partial<{ runs: number; wickets: number; extras: number; winner_team_code: string }>;
    errors: Partial<{ runs_mae: number; wickets_mae: number; extras_mae: number }>;
  };
  players: BacktestPlayerResult[];
  metrics: Partial<{ player_runs_mae: number; player_runs_rmse: number; winner_accuracy: number; player_wickets_mae: number }>;
};
