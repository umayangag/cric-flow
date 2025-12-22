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
  squads: SquadDTO[]; // exactly two
};
