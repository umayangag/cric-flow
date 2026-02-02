import Papa from 'papaparse';
import type { CsvRow, PlayerPrediction } from '../types';

export function parseCsv(text: string): CsvRow[] {
  const parsed = Papa.parse(text, {
    header: true,
    dynamicTyping: true,
    skipEmptyLines: true,
  });
  if (parsed.errors?.length) {
    const e = parsed.errors[0];
    throw new Error(`CSV parse error at row ${e.row}: ${e.message}`);
  }
  const rows = (parsed.data as unknown[]).map((r) => normalizeRow(r));
  return rows;
}

function normalizeRow(r: unknown): CsvRow {
  const obj = (r && typeof r === 'object' ? (r as Record<string, unknown>) : {}) as Record<
    string,
    unknown
  >;
  const dateStr = String(obj.date ?? '').slice(0, 10);
  const season = obj.season != null ? Number(obj.season) : inferSeasonFromDate(dateStr);
  const base: CsvRow = {
    match_id: (obj as Record<string, unknown>).match_id as string | number,
    date: dateStr,
    season,
    team_name: String(obj.team_name ?? ''),
    actual_win: Number(obj.actual_win),
    player_name: String(obj.player_name ?? ''),
    runs_scored: toNum(obj.runs_scored),
    balls_faced: toNum(obj.balls_faced),
    fours_scored: toNum(obj.fours_scored),
    sixes_scored: toNum(obj.sixes_scored),
    batting_position: toNum(obj.batting_position),
    strike_rate: toNum(obj.strike_rate),
    runs_conceded: toNum(obj.runs_conceded),
    deliveries: toNum(obj.deliveries),
    wickets_taken: toNum(obj.wickets_taken),
    econ: toNum(obj.econ),
    winning_probability: obj.winning_probability != null ? Number(obj.winning_probability) : null,
  };
  return base;
}

function toNum(v: unknown): number {
  const n = Number(v);
  return Number.isFinite(n) ? n : 0;
}

export function inferSeasonFromDate(dateStr: string): number {
  if (!dateStr) return 0;
  const y = Number(dateStr.slice(0, 4));
  return Number.isFinite(y) ? y : 0;
}

export type SquadKey = string; // `${match_id}__${team_name}`

export type Squad = {
  match_id: string | number;
  team_name: string;
  season: number;
  actual_win: number; // 0|1
  players: PlayerPrediction[];
};

export function buildSquads(rows: CsvRow[]): Map<SquadKey, Squad> {
  const m = new Map<SquadKey, Squad>();
  for (const r of rows) {
    const key = `${r.match_id}__${r.team_name}`;
    const existing = m.get(key);
    if (!existing) {
      m.set(key, {
        match_id: r.match_id,
        team_name: r.team_name,
        season: r.season ?? inferSeasonFromDate(r.date),
        actual_win: Number(r.actual_win),
        players: [pickPlayerFields(r)],
      });
    } else {
      existing.players.push(pickPlayerFields(r));
    }
  }
  return m;
}

function pickPlayerFields(r: CsvRow): PlayerPrediction {
  return {
    player_name: r.player_name,
    runs_scored: r.runs_scored,
    balls_faced: r.balls_faced,
    fours_scored: r.fours_scored,
    sixes_scored: r.sixes_scored,
    batting_position: r.batting_position,
    strike_rate: r.strike_rate,
    runs_conceded: r.runs_conceded,
    deliveries: r.deliveries,
    wickets_taken: r.wickets_taken,
    econ: r.econ,
    winning_probability: r.winning_probability ?? undefined,
  };
}

export function determineImmediateNextSeason(cutoffDate: string, seasons: number[]): number | null {
  const base = inferSeasonFromDate(cutoffDate);
  const uniqueSorted = Array.from(new Set(seasons)).sort((a, b) => a - b);
  for (const s of uniqueSorted) {
    if (s > base) return s;
  }
  return null;
}

export type EvalMetrics = {
  total: number;
  correct: number;
  accuracy: number;
  confusion: { tp: number; tn: number; fp: number; fn: number };
};

export function computeMetrics(
  preds: Array<{ prob: number; actual: number }>,
  threshold = 0.5,
): EvalMetrics {
  let tp = 0,
    tn = 0,
    fp = 0,
    fn = 0;
  for (const { prob, actual } of preds) {
    const yhat = prob >= threshold ? 1 : 0;
    if (yhat === 1 && actual === 1) tp++;
    else if (yhat === 0 && actual === 0) tn++;
    else if (yhat === 1 && actual === 0) fp++;
    else fn++;
  }
  const total = preds.length;
  const correct = tp + tn;
  const accuracy = total ? correct / total : 0;
  return { total, correct, accuracy, confusion: { tp, tn, fp, fn } };
}
