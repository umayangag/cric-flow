import type {
  HealthResponse,
  ModelMetadataResponse,
  ModelStatsResponse,
  BacktestSelectResponse,
  BacktestEvaluateResponse,
  EvaluateStatusResponse,
  MatchScorecardResponse,
  Migration,
  Suggestion,
  PaginatedResponse,
  PredictTeamSelectionResponse,
  PipelineRunResponse,
  PipelineProgressPayload,
  AccuracyTrendResponse,
  AccuracyTrendFilters,
} from './types';
import type { OpsStatusDTO } from './types';

const BASE_API_URL = (import.meta.env.VITE_API_URL as string) || 'http://localhost:8080';

/** Build headers for go-app API requests. Optionally omit Content-Type for GET/stream requests. */
function apiHeaders(includeJsonContentType = true): Record<string, string> {
  const headers: Record<string, string> = {};
  if (includeJsonContentType) headers['Content-Type'] = 'application/json';
  const apiKey = localStorage.getItem('cric_info_api_key');
  if (apiKey) headers['X-API-Key'] = apiKey;
  return headers;
}

// Generic HTTP client factory to avoid duplication between different base URLs
function createHttpClient(baseUrl: string) {
  return async function httpClient<T>(pathOrUrl: string, options?: RequestInit): Promise<T> {
    const url = pathOrUrl.startsWith('http') ? pathOrUrl : `${baseUrl}${pathOrUrl}`;

    // Improved detection: matches BASE_API_URL, has go-app port, or final URL has go-app port
    const isGoApp = baseUrl === BASE_API_URL || baseUrl.includes(':8080') || url.includes(':8080');
    const headers = isGoApp ? apiHeaders(true) : { 'Content-Type': 'application/json' };

    const res = await fetch(url, {
      headers,
      ...options,
    });
    if (!res.ok) {
      const text = await res.text().catch(() => '');
      throw new Error(`HTTP ${res.status} ${res.statusText}: ${text}`);
    }
    return (await res.json()) as T;
  };
}

// Specific clients
const httpApi = createHttpClient(BASE_API_URL);

type SSECallbacks = {
  onProgress: (step: string, message: string) => void;
  onResult: (result: BacktestEvaluateResponse) => void;
  onError: (err: Error) => void;
};

/** Process one SSE event part; returns 'result' or 'error' when the stream is done, 'continue' otherwise. */
function processBacktestSSEPart(
  part: string,
  callbacks: SSECallbacks,
): 'continue' | 'result' | 'error' {
  let eventType = '';
  let data = '';
  for (const line of part.split('\n')) {
    if (/^event:\s?/.test(line)) eventType = line.replace(/^event:\s?/, '').trim();
    else if (/^data:\s?/.test(line)) data = line.replace(/^data:\s?/, '');
  }
  if (eventType === 'progress' && data) {
    try {
      const { step, message } = JSON.parse(data) as { step: string; message: string };
      callbacks.onProgress(step ?? '', message ?? '');
    } catch {
      /* ignore */
    }
    return 'continue';
  }
  if (eventType === 'result' && data) {
    try {
      const result = JSON.parse(data) as BacktestEvaluateResponse;
      callbacks.onResult(result);
    } catch (e) {
      callbacks.onError(e instanceof Error ? e : new Error(String(e)));
    }
    return 'result';
  }
  if (eventType === 'error' && data) {
    try {
      const { message } = JSON.parse(data) as { message?: string };
      callbacks.onError(new Error(message ?? 'Unknown error'));
    } catch {
      callbacks.onError(new Error(data));
    }
    return 'error';
  }
  return 'continue';
}

export const api = {
  apiHealth(): Promise<{ status: string }> {
    return httpApi('/health');
  },
  /** ML service health (loaded formats, artifacts). Uses Go API proxy so the frontend gets full details. */
  health(): Promise<HealthResponse> {
    return httpApi('/api/health/ml');
  },
  /** Model metadata (features, outputs, artifacts pattern) from ml-service for Workbench UI. */
  getModelMetadata(): Promise<ModelMetadataResponse> {
    return httpApi('/api/ml/model-metadata');
  },
  /** Model stats (name, format, params, accuracy, size) from ml-service for ML Model Stats tab. */
  getModelStats(): Promise<ModelStatsResponse> {
    return httpApi('/api/ml/model-stats');
  },
  // --- Backtest API (select and evaluate) ---
  backtestSelect(format: string, team1: string, team2: string): Promise<BacktestSelectResponse> {
    const u = new URL('/api/backtest/match', BASE_API_URL);
    u.searchParams.set('format', format);
    u.searchParams.set('team1', team1);
    u.searchParams.set('team2', team2);
    // mode defaults to select when match_id absent
    return httpApi(u.toString());
  },
  backtestEvaluate(
    format: string,
    team1: string,
    team2: string,
    matchId: number | string,
  ): Promise<BacktestEvaluateResponse> {
    const u = new URL('/api/backtest/match', BASE_API_URL);
    u.searchParams.set('format', format);
    u.searchParams.set('team1', team1);
    u.searchParams.set('team2', team2);
    u.searchParams.set('mode', 'evaluate');
    u.searchParams.set('match_id', String(matchId));
    return httpApi(u.toString());
  },
  // --- Ops Status (go-app API) ---
  opsStatus(): Promise<OpsStatusDTO> {
    return httpApi('/ops/status');
  },
  opsMigrations(page = 1, limit = 10): Promise<PaginatedResponse<Migration>> {
    const u = new URL('/ops/migrations', BASE_API_URL);
    u.searchParams.set('page', String(page));
    u.searchParams.set('limit', String(limit));
    return httpApi(u.toString());
  },
  opsSuggestions(): Promise<Suggestion[]> {
    return httpApi('/ops/suggestions');
  },
  /**
   * Trigger a pipeline step (import, precompute, export, train_*, auto_tune).
   * For auto_tune, pass params: { model?, format?, all_formats? } (query string).
   * Returns status and body so UI can handle 202 (started), 501 (run from root), or error.
   */
  async opsPipelineRun(
    step: string,
    params?: Record<string, string>,
  ): Promise<{ status: number; data: PipelineRunResponse }> {
    let url = `${BASE_API_URL}/ops/pipeline/run/${encodeURIComponent(step)}`;
    if (params) {
      const validParams = Object.fromEntries(
        Object.entries(params).filter(([, v]) => v !== undefined && v !== ''),
      );
      if (Object.keys(validParams).length > 0) {
        const q = new URLSearchParams(validParams).toString();
        if (q) url += `?${q}`;
      }
    }
    const res = await fetch(url, { method: 'POST', headers: apiHeaders() });
    let data: PipelineRunResponse = {};
    try {
      const text = await res.text();
      if (text) data = JSON.parse(text) as PipelineRunResponse;
    } catch {
      data = { error: res.statusText || 'Invalid response' };
    }
    return { status: res.status, data };
  },
  /**
   * Stop the currently running pipeline step (POST /ops/pipeline/stop).
   * Returns 200 with { status: 'cancelled' } or 409 if no step is running.
   */
  async opsPipelineStop(): Promise<{ status: number; data: { status?: string; error?: string } }> {
    const url = `${BASE_API_URL}/ops/pipeline/stop`;
    const res = await fetch(url, { method: 'POST', headers: apiHeaders() });
    let data: { status?: string; error?: string } = {};
    try {
      const text = await res.text();
      if (text) data = JSON.parse(text) as { status?: string; error?: string };
    } catch {
      data = { error: res.statusText || 'Invalid response' };
    }
    return { status: res.status, data };
  },
  /**
   * Subscribe to pipeline progress SSE (GET /ops/pipeline/stream).
   * Calls onProgress with each event; runs until stream ends or signal aborts.
   */
  async subscribePipelineProgress(
    signal: AbortSignal,
    onProgress: (payload: PipelineProgressPayload) => void,
  ): Promise<void> {
    const url = `${BASE_API_URL}/ops/pipeline/stream`;
    const res = await fetch(url, { headers: apiHeaders(false), signal });
    if (!res.ok) {
      const text = await res.text().catch(() => '');
      throw new Error(`HTTP ${res.status}: ${text}`);
    }
    const reader = res.body?.getReader();
    if (!reader) throw new Error('No response body');
    const decoder = new TextDecoder();
    let buffer = '';
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const parts = buffer.split('\n\n');
        buffer = parts.pop() ?? '';
        for (const part of parts) {
          if (!part.trim()) continue;
          const dataLines = part.split('\n').filter((l) => /^data:\s/.test(l) || /^data:/.test(l));
          if (dataLines.length > 0) {
            // Per SSE spec: multiple data: lines for one event are joined with newline
            const data = dataLines.map((l) => l.replace(/^data:\s*/, '')).join('\n');
            try {
              const payload = JSON.parse(data) as PipelineProgressPayload;
              onProgress(payload);
            } catch (e) {
              console.error('Failed to parse pipeline progress SSE data:', e, 'Data:', data);
            }
          }
        }
      }
    } catch (e) {
      if ((e as { name?: string }).name === 'AbortError') return;
      throw e;
    }
  },
  // --- Options ---
  getTeams(): Promise<string[]> {
    return httpApi('/api/options/teams');
  },
  getTeamsByFormat(format: string): Promise<string[]> {
    const u = new URL('/api/options/teams-by-format', BASE_API_URL);
    u.searchParams.set('format', format);
    return httpApi(u.toString());
  },
  getOpponents(format: string, team: string): Promise<string[]> {
    const u = new URL('/api/options/opponents', BASE_API_URL);
    u.searchParams.set('format', format);
    u.searchParams.set('team', team);
    return httpApi(u.toString());
  },
  getFormats(): Promise<string[]> {
    return httpApi('/api/options/formats');
  },
  /** Search venues by query; returns empty array if query has fewer than 3 characters. */
  searchVenues(q: string): Promise<string[]> {
    const trimmed = (q ?? '').trim();
    if (trimmed.length < 3) return Promise.resolve([]);
    const u = new URL('/api/options/venues', BASE_API_URL);
    u.searchParams.set('q', trimmed);
    return httpApi(u.toString());
  },
  /** Match scorecard (innings, batting and bowling card) for evaluate DB tab. */
  getMatchScorecard(matchId: number): Promise<MatchScorecardResponse> {
    const u = new URL('/api/backtest/scorecard', BASE_API_URL);
    u.searchParams.set('match_id', String(matchId));
    return httpApi(u.toString());
  },

  /** Accuracy trend (backtest metrics over matches) for Workbench tab. */
  accuracyTrend(filters: AccuracyTrendFilters = {}): Promise<AccuracyTrendResponse> {
    const u = new URL('/api/backtest/accuracy-trend', BASE_API_URL);
    if (filters.format) u.searchParams.set('format', filters.format);
    if (filters.start_date) u.searchParams.set('start_date', filters.start_date);
    if (filters.end_date) u.searchParams.set('end_date', filters.end_date);
    if (filters.team1) u.searchParams.set('team1', filters.team1);
    if (filters.team2) u.searchParams.set('team2', filters.team2);
    if (filters.order) u.searchParams.set('order', filters.order);
    if (filters.limit != null && filters.limit > 0)
      u.searchParams.set('limit', String(filters.limit));
    if (filters.cache) u.searchParams.set('cache', filters.cache);
    if (filters.metrics) u.searchParams.set('metrics', filters.metrics);
    if (filters.use_unified_model === true) u.searchParams.set('use_unified_model', '1');
    return httpApi(u.toString());
  },

  /**
   * Evaluate with Server-Sent Events progress. Calls onProgress(step, message) for each step,
   * onResult(result) with the final response, or onError(err) on failure.
   */
  async backtestEvaluateStream(
    format: string,
    team1: string,
    team2: string,
    matchId: number | string,
    callbacks: {
      onProgress: (step: string, message: string) => void;
      onResult: (result: BacktestEvaluateResponse) => void;
      onError: (err: Error) => void;
    },
    options?: { use_unified_model?: boolean; use_latest_model?: boolean },
  ): Promise<void> {
    const u = new URL('/api/backtest/evaluate-stream', BASE_API_URL);
    u.searchParams.set('format', format);
    u.searchParams.set('team1', team1);
    u.searchParams.set('team2', team2);
    u.searchParams.set('match_id', String(matchId));
    if (options?.use_unified_model === true) u.searchParams.set('use_unified_model', '1');
    if (options?.use_latest_model === true) u.searchParams.set('use_latest_model', '1');
    const res = await fetch(u.toString(), { headers: apiHeaders(false) });
    if (!res.ok) {
      const text = await res.text().catch(() => '');
      callbacks.onError(new Error(`HTTP ${res.status}: ${text}`));
      return;
    }
    const reader = res.body?.getReader();
    if (!reader) {
      callbacks.onError(new Error('No response body'));
      return;
    }
    const decoder = new TextDecoder();
    let buffer = '';
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const parts = buffer.split('\n\n');
        buffer = parts.pop() ?? '';
        for (const part of parts) {
          if (!part.trim()) continue;
          const action = processBacktestSSEPart(part, callbacks);
          if (action === 'result' || action === 'error') return;
        }
      }
      if (buffer.trim()) {
        const action = processBacktestSSEPart(buffer, callbacks);
        if (action === 'result' || action === 'error') return;
      }
    } catch (e) {
      callbacks.onError(e instanceof Error ? e : new Error(String(e)));
      return;
    }
    callbacks.onError(new Error('Stream ended without result'));
  },

  /**
   * Start evaluation in the background. Returns job_id; poll getEvaluateStatus(job_id) for progress and result.
   * Survives page refresh: store job_id and poll on load to restore state.
   */
  evaluateStart(
    format: string,
    team1: string,
    team2: string,
    matchId: number | string,
    options?: { use_unified_model?: boolean; use_latest_model?: boolean },
  ): Promise<{ job_id: string }> {
    const u = new URL('/api/backtest/evaluate-start', BASE_API_URL);
    u.searchParams.set('format', format);
    u.searchParams.set('team1', team1);
    u.searchParams.set('team2', team2);
    u.searchParams.set('match_id', String(matchId));
    if (options?.use_unified_model === true) u.searchParams.set('use_unified_model', '1');
    if (options?.use_latest_model === true) u.searchParams.set('use_latest_model', '1');

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 30000); // 30s timeout

    return fetch(u.toString(), {
      method: 'POST',
      headers: apiHeaders(false),
      signal: controller.signal,
    })
      .then(async (res) => {
        if (!res.ok) {
          const text = await res.text().catch(() => '');
          throw new Error(`HTTP ${res.status}: ${text}`);
        }
        return res.json() as Promise<{ job_id: string }>;
      })
      .finally(() => {
        clearTimeout(timeoutId);
      });
  },

  /** Get current status of an evaluation job (running / done / error). Poll until status is done or error. */
  getEvaluateStatus(jobId: string): Promise<EvaluateStatusResponse> {
    const u = new URL('/api/backtest/evaluate-status', BASE_API_URL);
    u.searchParams.set('job_id', jobId);
    return httpApi(u.toString());
  },

  /**
   * Predict best 11 for each team for an upcoming match.
   * Requires future date within max limit (e.g. 2 weeks) for accurate predictions.
   */
  predictTeamSelection(params: {
    format: string;
    team1: string;
    team2: string;
    venue?: string;
    match_date: string; // YYYY-MM-DD or RFC3339
    season_id?: number;
    extra_team1?: number[];
    extra_team2?: number[];
    min_bowlers?: number;
    require_keeper?: boolean;
    use_unified_model?: boolean;
  }): Promise<PredictTeamSelectionResponse> {
    return httpApi('/api/predict/team-selection', {
      method: 'POST',
      body: JSON.stringify(params),
    });
  },
};
