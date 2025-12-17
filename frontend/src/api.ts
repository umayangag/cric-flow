import type { HealthResponse, PlayerPrediction, TeamWinResponse } from './types';

const BASE_URL = (import.meta.env.VITE_ML_SERVICE_URL as string) || 'http://localhost:8000';

async function http<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  });
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    throw new Error(`HTTP ${res.status} ${res.statusText}: ${text}`);
  }
  return (await res.json()) as T;
}

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
};
