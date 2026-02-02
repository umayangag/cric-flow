// Accuracy Trend API client and TypeScript interfaces

export interface AccuracyTrendItem {
  match_id: number;
  date: string;
  format: string;
  team1: string;
  team2: string;
  metrics: Record<string, number>;
}

export interface AccuracyTrendResponse {
  filters: Record<string, unknown>;
  count: number;
  results: AccuracyTrendItem[];
  summary: Record<string, number>;
  progressive: Array<Record<string, number>>;
}

export type CacheMode = 'off' | 'read' | 'readwrite';

export interface AccuracyTrendFilters {
  format?: string;
  start_date?: string; // YYYY-MM-DD
  end_date?: string; // YYYY-MM-DD
  team1?: string;
  team2?: string;
  order?: 'asc' | 'desc';
  limit?: number; // default enforced by backend (100), cap 500
  cache?: CacheMode; // default readwrite
  metrics?: string; // comma-separated: player,team
}

function buildAccuracyTrendUrl(
  params: Partial<AccuracyTrendFilters> & { baseUrl?: string } = {},
): string {
  const { baseUrl, ...query } = params;

  // Determine base endpoint
  let endpoint = '/api/backtest/accuracy-trend';
  const envBase =
    typeof process !== 'undefined' &&
    typeof (process as unknown as { env?: Record<string, unknown> })?.env?.NEXT_PUBLIC_API_BASE ===
      'string'
      ? String((process as unknown as { env?: Record<string, unknown> }).env!.NEXT_PUBLIC_API_BASE)
      : undefined;
  if (baseUrl) {
    endpoint = `${baseUrl.replace(/\/$/, '')}/api/backtest/accuracy-trend`;
  } else if (envBase) {
    endpoint = `${String(envBase).replace(/\/$/, '')}/api/backtest/accuracy-trend`;
  }

  const qs = new URLSearchParams();
  const add = (k: string, v: unknown) => {
    if (v === undefined || v === null) return;
    const s = String(v).trim();
    if (s.length === 0) return;
    qs.append(k, s);
  };

  add('format', query.format);
  add('team1', query.team1);
  add('team2', query.team2);
  add('start_date', query.start_date);
  add('end_date', query.end_date);
  add('order', query.order);
  if (typeof query.limit === 'number' && Number.isFinite(query.limit) && query.limit > 0) {
    add('limit', Math.floor(query.limit));
  }
  add('cache', query.cache);
  add('metrics', query.metrics);

  const qsStr = qs.toString();
  return qsStr ? `${endpoint}?${qsStr}` : endpoint;
}

export async function fetchAccuracyTrend(
  params: Partial<AccuracyTrendFilters> & { baseUrl?: string } = {},
): Promise<AccuracyTrendResponse> {
  const url = buildAccuracyTrendUrl(params);
  const res = await fetch(url, { method: 'GET' });
  if (!res.ok) {
    let body: string;
    try {
      body = await res.text();
    } catch {
      body = '';
    }
    throw new Error(
      `accuracy-trend request failed: HTTP ${res.status} ${res.statusText}${body ? ` — ${body}` : ''}`,
    );
  }
  const json = (await res.json()) as AccuracyTrendResponse;
  return json;
}

export const AccuracyTrendApi = {
  buildUrl: buildAccuracyTrendUrl,
  fetch: fetchAccuracyTrend,
};
