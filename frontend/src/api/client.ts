import type { BacktestSelectResponse, BacktestEvaluateResponse } from './types';
import type { Migration, Suggestion } from '../types';

type SelectParams = { format: string; team1: string; team2: string };
type EvaluateParams = {
  format: string;
  team1: string;
  team2: string;
  matchId: number;
  cutoffUtcIso: string; // RFC3339 (UTC Z recommended)
  useML?: boolean; // default true
};

export function toUpperTrim(s: string): string {
  return (s || '').trim().toUpperCase();
}

export function isRFC3339(s: string): boolean {
  // Basic RFC3339 check; accepts time zone Z or ±hh:mm
  // Example: 2024-10-30T14:00:00Z
  const re = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;
  return re.test(s);
}

export function buildSelectQuery(params: SelectParams): string {
  const format = toUpperTrim(params.format);
  const team1 = toUpperTrim(params.team1);
  const team2 = toUpperTrim(params.team2);
  if (!format || !team1 || !team2) {
    throw new Error('format, team1, team2 are required');
  }
  const qp = new URLSearchParams({ format, team1, team2, mode: 'select' });
  return `/api/backtest/match?${qp.toString()}`;
}

export function buildEvaluateQuery(params: EvaluateParams): string {
  const format = toUpperTrim(params.format);
  const team1 = toUpperTrim(params.team1);
  const team2 = toUpperTrim(params.team2);
  const matchId = Number(params.matchId);
  const useML = params.useML !== false; // default true
  if (!format || !team1 || !team2) {
    throw new Error('format, team1, team2 are required');
  }
  if (!Number.isFinite(matchId) || matchId <= 0) {
    throw new Error('matchId must be a positive number');
  }
  const qp: Record<string, string> = {
    format,
    team1,
    team2,
    mode: 'evaluate',
    match_id: String(matchId),
  };
  if (useML) {
    const cutoff = (params.cutoffUtcIso || '').trim();
    if (!cutoff) {
      throw new Error('cutoffUtcIso (RFC3339) is required when useML is true');
    }
    if (!isRFC3339(cutoff)) {
      throw new Error('cutoffUtcIso must be RFC3339, e.g., 2024-10-30T14:00:00Z');
    }
    qp['use_ml'] = '1';
    qp['cutoff'] = cutoff;
  }
  const search = new URLSearchParams(qp);
  return `/api/backtest/match?${search.toString()}`;
}

export async function fetchBacktestSelect(
  baseUrl: string,
  params: SelectParams,
  init?: RequestInit,
): Promise<BacktestSelectResponse> {
  const url = (baseUrl || '') + buildSelectQuery(params);
  const res = await fetch(url, { ...init, method: 'GET' });
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    throw new Error(`HTTP ${res.status} ${res.statusText}: ${text}`);
  }
  return (await res.json()) as BacktestSelectResponse;
}

export async function fetchBacktestEvaluate(
  baseUrl: string,
  params: EvaluateParams,
  init?: RequestInit,
): Promise<BacktestEvaluateResponse> {
  const url = (baseUrl || '') + buildEvaluateQuery(params);
  const res = await fetch(url, { ...init, method: 'GET' });
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    throw new Error(`HTTP ${res.status} ${res.statusText}: ${text}`);
  }
  return (await res.json()) as BacktestEvaluateResponse;
}

export async function fetchOpsMigrations(baseUrl: string): Promise<Migration[]> {
  const res = await fetch((baseUrl || '') + '/ops/migrations');
  if (!res.ok) {
    throw new Error(`HTTP ${res.status} ${res.statusText}`);
  }
  return res.json();
}

export async function fetchOpsSuggestions(baseUrl: string): Promise<Suggestion[]> {
  const res = await fetch((baseUrl || '') + '/ops/suggestions');
  if (!res.ok) {
    throw new Error(`HTTP ${res.status} ${res.statusText}`);
  }
  return res.json();
}
