import type {
  HealthResponse,
  PlayerPrediction,
  TeamWinResponse,
  SeasonsNextResponse,
  MatchListItem,
  MatchSquadsResponse,
} from './types';

const BASE_URL = import.meta.env.VITE_ML_SERVICE_URL || 'http://localhost:8000';
const BASE_API_URL = (import.meta.env.VITE_API_URL as string) || 'http://localhost:8080';

// Generic HTTP client factory to avoid duplication between different base URLs
function createHttpClient(baseUrl: string) {
  return async function httpClient<T>(pathOrUrl: string, options?: RequestInit): Promise<T> {
    const url = pathOrUrl.startsWith('http') ? pathOrUrl : `${baseUrl}${pathOrUrl}`;
    const res = await fetch(url, {
      headers: { 'Content-Type': 'application/json' },
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
const http = createHttpClient(BASE_URL);
const httpApi = createHttpClient(BASE_API_URL);

export const api = {
  health(): Promise<HealthResponse> {
    return http('/health');
  },
  predictWin(players: PlayerPrediction[]): Promise<TeamWinResponse> {
    return http('/predict/win', {
      method: 'POST',
      body: JSON.stringify(players),
    });
  },
  // --- DB-backed endpoints (go-app API) ---
  seasonsNext(cutoff: string, format?: string): Promise<SeasonsNextResponse> {
    const u = new URL('/seasons/next', BASE_API_URL);
    u.searchParams.set('cutoff', cutoff);
    if (format) u.searchParams.set('format', format);
    return httpApi(u.toString());
  },
  listMatches(season: number, after: string, format?: string): Promise<MatchListItem[]> {
    const u = new URL('/matches', BASE_API_URL);
    u.searchParams.set('season', String(season));
    u.searchParams.set('after', after);
    if (format) u.searchParams.set('format', format);
    return httpApi(u.toString());
  },
  getMatchSquads(matchId: number | string, asof: string, format?: string): Promise<MatchSquadsResponse> {
    const u = new URL(`/match/${matchId}/squads`, BASE_API_URL);
    u.searchParams.set('asof', asof);
    if (format) u.searchParams.set('format', format);
    return httpApi(u.toString());
  },
};
