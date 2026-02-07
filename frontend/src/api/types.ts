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
  filters: Record<string, unknown>;
  match: { match_id: number; date: string };
  match_aggregates?: {
    predicted: Record<string, unknown>;
    actual: Record<string, unknown>;
    errors: Record<string, number>;
  };
  players: BacktestPlayerResult[];
  metrics: Record<string, number>;
};
