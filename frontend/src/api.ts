import { ApiError } from './lib/apiError';
import type {
  HealthResponse,
  XiStatusResponse,
  EvaluationReport,
  TrackRecord,
  MetricGlossary,
  Migration,
  Suggestion,
  PaginatedResponse,
  PredictTeamSelectionResponse,
  PipelineRunResponse,
  PipelineProgressPayload,
  PipelineLane,
  BiographyCoverageResponse,
  DatasetRegistryResponse,
  DataFeedsResponse,
  StagedResponse,
  OpsDataStartResponse,
  RunPlanState,
  RunPlanStartResponse,
  TeamSideOption,
  PipelineStopResult,
  CandidatesResponse,
  PoolRequest,
  RetirementStatus,
  AuctionPlayerState,
  AuctionResponse,
  AuctionSummary,
  AuctionProjection,
  AuctionOppositionSuggestion,
  AuctionVenueWeight,
  PlayerSearchResult,
} from './types';
import type { OpsStatusDTO } from './types';

const BASE_API_URL = (import.meta.env.VITE_API_URL as string) || 'http://localhost:8080';

/** Build headers for go-app API requests. Optionally omit Content-Type for GET/stream requests. */
function apiHeaders(includeJsonContentType = true): Record<string, string> {
  const headers: Record<string, string> = {};
  if (includeJsonContentType) headers['Content-Type'] = 'application/json';
  let apiKey: string | null = null;
  if (typeof window !== 'undefined' && window.localStorage) {
    apiKey = window.localStorage.getItem('cric_info_api_key');
  }
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
      throw await readApiError(res);
    }
    return (await res.json()) as T;
  };
}

/**
 * Turn a failed response into an ApiError, keeping the parts the backend wrote.
 *
 * This used to be `throw new Error('HTTP 400 Bad Request: ' + text)`, which is why no
 * surface in the app showed a hint: there was nothing to show it from. go-app answers
 * `{code, message, hint?, available?}` and ml-service wraps the same shape in
 * `{detail: …}`; both are read here, and anything unrecognised falls back to the raw
 * text so a proxy's HTML error page is still legible rather than swallowed.
 */
async function readApiError(res: Response): Promise<ApiError> {
  const text = await res.text().catch(() => '');
  const fallback = `HTTP ${res.status} ${res.statusText}`.trim();

  let parsed: unknown;
  try {
    parsed = JSON.parse(text) as unknown;
  } catch {
    return new ApiError(text.trim() || fallback, { status: res.status });
  }

  const envelope = parsed as { detail?: unknown; error?: unknown };
  const payload = (
    envelope && typeof envelope.detail === 'object' && envelope.detail !== null
      ? envelope.detail
      : parsed
  ) as {
    code?: unknown;
    message?: unknown;
    hint?: unknown;
    available?: unknown;
    error?: unknown;
  };

  // `error` rather than `message` is the older data-job shape; both are the message.
  const message =
    stringOrUndefined(payload?.message) ?? stringOrUndefined(payload?.error) ?? fallback;

  return new ApiError(message, {
    status: res.status,
    code: stringOrUndefined(payload?.code),
    hint: stringOrUndefined(payload?.hint),
    available: Array.isArray(payload?.available)
      ? payload.available.filter((v): v is string => typeof v === 'string')
      : undefined,
  });
}

function stringOrUndefined(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() !== '' ? value : undefined;
}

// Specific clients
const httpApi = createHttpClient(BASE_API_URL);

/**
 * POST a dataset job and return status alongside body.
 *
 * These endpoints answer 202 on success and carry a usable message on 400 (source
 * refused, nothing staged) and 409 (a data step already running). Throwing on
 * non-2xx would discard exactly the part the operator needs to read — including
 * `allowed_hosts`, which is what turns "rejected" into "here is what is allowed".
 */
async function postDataJob(
  path: string,
  body: Record<string, string | undefined>,
): Promise<{ status: number; data: OpsDataStartResponse }> {
  const url = `${BASE_API_URL}${path}`;
  const res = await fetch(url, {
    method: 'POST',
    headers: apiHeaders(),
    body: JSON.stringify(body),
  });
  let data: OpsDataStartResponse = {};
  try {
    const text = await res.text();
    if (text) data = JSON.parse(text) as OpsDataStartResponse;
  } catch {
    data = { error: res.statusText || 'Invalid response' };
  }
  return { status: res.status, data };
}

export const api = {
  apiHealth(): Promise<{ status: string }> {
    return httpApi('/health');
  },
  /** ML service health (loaded formats, artifacts). Uses Go API proxy so the frontend gets full details. */
  health(): Promise<HealthResponse> {
    return httpApi('/api/health/ml');
  },
  /** Which run is loaded, what its manifest records, and whether its ratings are fresh. */
  xiStatus(): Promise<XiStatusResponse> {
    return httpApi('/api/ml/xi-status');
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
   * The latest run plan (GET /ops/pipeline/plan), running or not.
   *
   * Not "current": the state lives in the database, so this returns a plan the page
   * did not start — after a reload, from another tab, or the morning after.
   */
  opsRunPlan(options?: { signal?: AbortSignal }): Promise<RunPlanState> {
    return httpApi('/ops/pipeline/plan', { signal: options?.signal });
  },
  /**
   * Start or resume a run plan (POST /ops/pipeline/run-plan).
   *
   * `resume` continues the last plan, skipping the steps it completed. Without it
   * every step in the plan runs.
   *
   * Returns status alongside body rather than throwing, so 409 ("a plan is already in
   * progress", "nothing to resume") reads as the answer it is rather than a crash.
   */
  async opsRunPlanStart(body: {
    plan?: string;
    steps?: string[];
    resume?: boolean;
  }): Promise<{ status: number; data: RunPlanStartResponse }> {
    const url = `${BASE_API_URL}/ops/pipeline/run-plan`;
    const res = await fetch(url, {
      method: 'POST',
      headers: apiHeaders(),
      body: JSON.stringify(body),
    });
    let data: RunPlanStartResponse = {};
    try {
      const text = await res.text();
      if (text) data = JSON.parse(text) as RunPlanStartResponse;
    } catch {
      data = { error: res.statusText || 'Invalid response' };
    }
    return { status: res.status, data };
  },
  /** Named feeds and the host allowlist (GET /ops/data/feeds). */
  opsDataFeeds(options?: { signal?: AbortSignal }): Promise<DataFeedsResponse> {
    return httpApi('/ops/data/feeds', { signal: options?.signal });
  },
  /** Archives waiting to be extracted, plus the live dataset's manifest (GET /ops/data/staged). */
  opsDataStaged(options?: { signal?: AbortSignal }): Promise<StagedResponse> {
    return httpApi('/ops/data/staged', { signal: options?.signal });
  },
  /**
   * Start a dataset download (POST /ops/data/fetch). Pass a feed id or an explicit
   * URL, never both — the backend refuses the ambiguity rather than picking one.
   *
   * Returns status and body rather than throwing, so the caller can tell 202
   * (started) from 400 (off the allowlist) from 409 (a data step already running)
   * and say something useful about each.
   */
  async opsDataFetch(body: {
    feed?: string;
    url?: string;
  }): Promise<{ status: number; data: OpsDataStartResponse }> {
    return postDataJob('/ops/data/fetch', body);
  },
  /**
   * Start an extraction (POST /ops/data/extract). Omit `archive` for the newest
   * staged archive.
   */
  async opsDataExtract(body: {
    archive?: string;
  }): Promise<{ status: number; data: OpsDataStartResponse }> {
    return postDataJob('/ops/data/extract', body);
  },
  /**
   * The dataset registry (GET /ops/data/datasets), newest first.
   *
   * `live` marks the dataset currently in the data directory. It is derived from that
   * directory's manifest rather than stored, so it stays correct when someone puts
   * files there by other means — in which case `live_sha256` is set and no entry is
   * marked live, meaning "the box holds a dataset this registry has never seen".
   */
  opsDatasets(limit = 50, options?: { signal?: AbortSignal }): Promise<DatasetRegistryResponse> {
    const u = new URL('/ops/data/datasets', BASE_API_URL);
    u.searchParams.set('limit', String(limit));
    return httpApi(u.toString(), { signal: options?.signal });
  },
  /**
   * Biography coverage (GET /ops/data/biography-coverage): how much of the archive the
   * acquired player biographies cover, per format and gender, weighted by appearances.
   *
   * It is measured on every call rather than stored, for the same reason the dataset
   * registry derives "live" from disk: a stored coverage figure goes stale the moment
   * an import adds players, and a stale figure is worse than none.
   */
  opsBiographyCoverage(options?: { signal?: AbortSignal }): Promise<BiographyCoverageResponse> {
    return httpApi(new URL('/ops/data/biography-coverage', BASE_API_URL).toString(), {
      signal: options?.signal,
    });
  },
  /**
   * Trigger a pipeline step (import, retrain, evaluate, reload).
   * Params go on the query string: `cutoff` for retrain and evaluate, `run_id` for reload,
   * `refresh` for import.
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
   * Stop running pipeline steps (POST /ops/pipeline/stop).
   *
   * With no lane it stops everything, which is what the Stop button means. Passing a
   * lane stops only that one — the lanes overlap by design, so cancelling a download
   * should not have to abandon a training run that is eight minutes in.
   *
   * Returns 200 with { status: 'cancelled', cancelled: n, training_stopped: [...] }, 409
   * if nothing is running, or 502 with `status: 'partially_cancelled'` when this run was
   * cancelled here but ml-service could not confirm its training process stopped — the
   * case that used to be reported as a plain success while a retrain kept running (D-11).
   */
  async opsPipelineStop(
    lane?: PipelineLane,
  ): Promise<{ status: number; data: PipelineStopResult }> {
    const query = lane ? `?lane=${encodeURIComponent(lane)}` : '';
    const url = `${BASE_API_URL}/ops/pipeline/stop${query}`;
    const res = await fetch(url, { method: 'POST', headers: apiHeaders() });
    let data: PipelineStopResult = {};
    try {
      const text = await res.text();
      if (text) data = JSON.parse(text) as PipelineStopResult;
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
  /**
   * The sides that have played a format, each with the `club_id` a prediction is requested
   * with. Not names: a name is not a team (D-10).
   */
  getTeamSidesByFormat(format: string): Promise<TeamSideOption[]> {
    const u = new URL('/api/options/teams-by-format', BASE_API_URL);
    u.searchParams.set('format', format);
    return httpApi(u.toString());
  },
  /** The sides this club has played in the format, addressed by its club id. */
  getOpponentSides(format: string, teamId: number): Promise<TeamSideOption[]> {
    const u = new URL('/api/options/opponents', BASE_API_URL);
    u.searchParams.set('format', format);
    u.searchParams.set('team_id', String(teamId));
    return httpApi(u.toString());
  },
  getFormats(): Promise<string[]> {
    return httpApi('/api/options/formats');
  },
  /** Canonical format codes (TEST, ODI, T20, T20I) from go-app. Use for ops grids and any UI that must match backend. */
  getCanonicalFormats(): Promise<string[]> {
    return httpApi('/api/canonical/formats');
  },
  /** Search venues by query; returns empty array if query has fewer than 3 characters. */
  searchVenues(q: string): Promise<string[]> {
    const trimmed = (q ?? '').trim();
    if (trimmed.length < 3) return Promise.resolve([]);
    const u = new URL('/api/options/venues', BASE_API_URL);
    u.searchParams.set('q', trimmed);
    return httpApi(u.toString());
  },
  /** L4's evaluation report, as `make evaluate` last wrote it. */
  evaluationReport(): Promise<EvaluationReport> {
    return httpApi('/api/backtest/report');
  },

  /**
   * The track record (P2-4): every stored prediction in its state, the scored ones against
   * what happened, computed on this read. There is nothing to configure and no page: the
   * states are decided over the whole record.
   */
  trackRecord(): Promise<TrackRecord> {
    return httpApi('/api/track-record');
  },

  /**
   * What every reported metric means (L-1), from the service that computes them.
   *
   * Its own endpoint rather than a read of the evaluation report: the prediction surfaces
   * show metrics and never load that report, and the report is a megabyte of folds nobody
   * needs in order to explain the word "pinball".
   */
  metricGlossary(): Promise<MetricGlossary> {
    return httpApi('/api/backtest/metric-glossary');
  },

  /**
   * Predict both XIs for an upcoming match: the selection, the displayed win probability
   * with its source, and -- where the format has an innings length -- the drawn scorecard
   * the per-player points and ranges come from.
   */
  predictTeamSelection(params: {
    format: string;
    /** The `club_id` of each side, from {@link getTeamSidesByFormat}. A name would not say
     * which of two teams it meant, and the API refuses an ambiguous one (D-10). */
    team1_id: number;
    team2_id: number;
    venue?: string;
    match_date: string; // YYYY-MM-DD or RFC3339
    extra_team1?: number[];
    extra_team2?: number[];
    min_bowlers?: number;
    require_keeper?: boolean;
    /**
     * The toss (P1-1): true where team 1 bats first, false where team 2 does. Omitted where
     * it is unknown, which is what the simulator marginalises over.
     */
    team1_bats_first?: boolean;
    /** Each side's candidate pool (D-12). Omitted, both get the per-format recency window. */
    team1_pool?: PoolRequest;
    team2_pool?: PoolRequest;
    /**
     * Play mode (P1-2): each side's eleven, by player id. Sent, the API scores exactly
     * these players and searches for nothing; omitted, it selects as it always has. Both
     * sides or neither -- searching one side while the user edits the other would move
     * numbers nobody touched.
     */
    team1_xi?: number[];
    team2_xi?: number[];
  }): Promise<PredictTeamSelectionResponse> {
    return httpApi('/api/predict/team-selection', {
      method: 'POST',
      body: JSON.stringify(params),
    });
  },

  /**
   * The candidate list a manual pool is ticked out of (D-12).
   *
   * It returns the players the pool would offer *and* the ones the retirement ledger is
   * keeping out, marked with the reason — an exclusion a user cannot see is one they
   * cannot undo.
   */
  getCandidates(params: {
    format: string;
    club_id: number;
    match_date?: string;
    window_months?: number;
    all_time?: boolean;
  }): Promise<CandidatesResponse> {
    const url = new URL('/api/options/candidates', BASE_API_URL);
    url.searchParams.set('format', params.format);
    url.searchParams.set('club_id', String(params.club_id));
    if (params.match_date) url.searchParams.set('match_date', params.match_date);
    if (params.window_months) url.searchParams.set('window_months', String(params.window_months));
    if (params.all_time) url.searchParams.set('all_time', 'true');
    return httpApi(url.toString());
  },

  /**
   * Flag a player retired.
   *
   * The claim hides him from this user's default pools straight away; it becomes the
   * stored `is_retired` fact only where an independent criterion corroborates it, and the
   * response says which one did — or which checks could not be made.
   */
  flagRetirement(playerId: number, format?: string): Promise<RetirementStatus> {
    const url = new URL(`/api/players/${playerId}/retirement`, BASE_API_URL);
    if (format) url.searchParams.set('format', format);
    return httpApi(url.toString(), { method: 'POST' });
  },

  /** Withdraw a retirement flag, demoting the stored fact it had raised. */
  unflagRetirement(playerId: number): Promise<RetirementStatus> {
    return httpApi(`/api/players/${playerId}/retirement`, { method: 'DELETE' });
  },

  // --- The auction record (P3-1) ---
  //
  // Every write answers with the auction as it now stands, so nothing here merges a
  // response into a local copy of the record: the answer *is* the record, and a client
  // that patched its own would be one mistyped entry from showing a squad the backend
  // does not hold. Nothing on these routes reaches /xi/optimize, returns a win probability
  // or returns a marginal value — the module is valuation, never XI-picking (plan §8.8).

  /** The index an operator finds an auction from after a reload. */
  auctions(): Promise<{ auctions: AuctionSummary[] }> {
    return httpApi('/api/auctions');
  },

  /** Set up an auction: its format, the buying side, the grounds and the squad to fill. */
  createAuction(body: {
    name: string;
    format: string;
    buyer_club_id: number;
    venue_ids?: number[];
    squad_size: number;
    min_bowlers?: number;
    require_keeper?: boolean;
  }): Promise<AuctionResponse> {
    return httpApi('/api/auctions', { method: 'POST', body: JSON.stringify(body) });
  },

  /** One auction whole, with the roles read off the served vectors. */
  auction(auctionId: string): Promise<AuctionResponse> {
    return httpApi(`/api/auctions/${auctionId}`);
  },

  /** List players onto the auction. A player already listed keeps the state he is in. */
  addAuctionPlayers(auctionId: string, playerIds: number[]): Promise<AuctionResponse> {
    return httpApi(`/api/auctions/${auctionId}/players`, {
      method: 'POST',
      body: JSON.stringify({ player_ids: playerIds }),
    });
  },

  /**
   * Record what the room did: a sale with its buyer and price, an unsold result, or an
   * undo back to available. The undo is the same call with the available state, because
   * the operator mistyped and the record now says he is available again.
   */
  recordAuctionOutcome(
    auctionId: string,
    outcome: {
      player_id: number;
      state: AuctionPlayerState;
      buyer_name?: string;
      buyer_club_id?: number;
      price?: number;
    },
  ): Promise<AuctionResponse> {
    return httpApi(`/api/auctions/${auctionId}/outcomes`, {
      method: 'POST',
      body: JSON.stringify(outcome),
    });
  },

  // --- The projection (P3-2) ---
  //
  // The two writes below the search are the projection's assumptions — an eleven the
  // operator guesses and an opposition they name — held on the record so every later item
  // reads one list. The projection itself carries no win probability and no marginal
  // value: go-app's own type for the match-drawing call has no such field, so one cannot
  // reach here. (The endpoint's name is not spelled here because the contract's rejected
  // body params are matched against this file as plain words.)

  /**
   * Name the projection's assumptions. Each field is optional and an absent one is left as
   * it stands: the operator names the opposition once and edits the likely eleven all
   * through the auction as their squad fills.
   */
  setAuctionAssumptions(
    auctionId: string,
    body: { likely_xi?: number[]; opposition?: { club_id: number; player_ids: number[] } },
  ): Promise<AuctionResponse> {
    return httpApi(`/api/auctions/${auctionId}/assumptions`, {
      method: 'PUT',
      body: JSON.stringify(body),
    });
  },

  /**
   * The eleven a side last fielded in this format, as a starting point to edit. A fact
   * with a date on it, never a side assembled by rating.
   */
  auctionOppositionSuggestion(
    auctionId: string,
    clubId: number,
  ): Promise<AuctionOppositionSuggestion> {
    return httpApi(`/api/auctions/${auctionId}/opposition-suggestion?club_id=${clubId}`);
  },

  /**
   * Project one candidate in the likely eleven, per ground. `team1_bats_first` omitted is
   * the toss unknown, which is the honest default months before a fixture exists;
   * `venue_weights` asks for a mixture over the named grounds and is omitted otherwise,
   * because how often an eleven plays where is a fact nobody has entered.
   */
  projectAuctionCandidate(
    auctionId: string,
    body: {
      player_id: number;
      team1_bats_first?: boolean | null;
      venue_weights?: AuctionVenueWeight[];
    },
  ): Promise<AuctionProjection> {
    return httpApi(`/api/auctions/${auctionId}/projection`, {
      method: 'POST',
      body: JSON.stringify(body),
    });
  },

  /**
   * The cross-club player search an auction list is built from.
   *
   * `getCandidates` is per club because a prediction is about one side; an auction room is
   * not one side.
   */
  searchPlayers(params: { q: string; format?: string; limit?: number }): Promise<{
    players: PlayerSearchResult[];
  }> {
    const url = new URL('/api/players/search', BASE_API_URL);
    url.searchParams.set('q', params.q);
    if (params.format) url.searchParams.set('format', params.format);
    if (params.limit) url.searchParams.set('limit', String(params.limit));
    return httpApi(url.toString());
  },
};
