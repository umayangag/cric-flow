import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  buildSelectQuery,
  buildEvaluateQuery,
  fetchBacktestSelect,
  fetchBacktestEvaluate,
} from './client';

declare global {
  // eslint-disable-next-line no-var
  var fetch: typeof fetch;
}

describe('api/client query builders', () => {
  it('buildSelectQuery builds select URL and uppercases params', () => {
    const url = buildSelectQuery({ format: 't20', team1: 'ind', team2: 'aus' });
    expect(url).toContain('/api/backtest/match?');
    expect(url).toContain('format=T20');
    expect(url).toContain('team1=IND');
    expect(url).toContain('team2=AUS');
    expect(url).toContain('mode=select');
  });

  it('buildEvaluateQuery builds evaluate URL with ML delegation and cutoff', () => {
    const url = buildEvaluateQuery({
      format: 'odi',
      team1: 'sl',
      team2: 'pak',
      matchId: 789,
      cutoffUtcIso: '2024-10-30T14:00:00Z',
    });
    expect(url).toContain('format=ODI');
    expect(url).toContain('team1=SL');
    expect(url).toContain('team2=PAK');
    expect(url).toContain('mode=evaluate');
    expect(url).toContain('match_id=789');
    expect(url).toContain('use_ml=1');
    expect(url).toContain('cutoff=2024-10-30T14%3A00%3A00Z');
  });

  it('buildEvaluateQuery errors when cutoff missing under ML', () => {
    expect(() =>
      buildEvaluateQuery({
        format: 't20',
        team1: 'ind',
        team2: 'aus',
        matchId: 1,
        cutoffUtcIso: '',
      }),
    ).toThrow(/cutoffUtcIso/);
  });
});

describe('api/client fetch helpers', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('fetchBacktestSelect fetches and parses JSON', async () => {
    const payload = { filters: {}, candidates: [{ match_id: 1, stable_id: '', date: '', venue: '', season: '', format: 'T20', team1: 'IND', team2: 'AUS', winner_team_code: '' }] };
    const mockFetch = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(payload) });
    // @ts-expect-error override global
    global.fetch = mockFetch;
    const res = await fetchBacktestSelect('', { format: 'T20', team1: 'IND', team2: 'AUS' });
    expect(res).toEqual(payload);
    expect(mockFetch).toHaveBeenCalledTimes(1);
    const calledUrl = mockFetch.mock.calls[0][0] as string;
    expect(calledUrl).toContain('/api/backtest/match');
  });

  it('fetchBacktestEvaluate fetches and parses JSON', async () => {
    const payload = {
      filters: { delegated: true },
      match: { match_id: 789, date: '2024-10-30T14:00:00Z' },
      match_aggregates: {
        predicted: { runs: 160 },
        actual: { runs: 155 },
        errors: { runs_mae: 5 },
      },
      players: [
        { player_id: 101, predicted: { runs: 20 }, actual: { runs: 18 }, errors: { runs_mae: 2 } },
      ],
      metrics: { player_runs_mae: 2 },
    };
    const mockFetch = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(payload) });
    // @ts-expect-error override global
    global.fetch = mockFetch;
    const res = await fetchBacktestEvaluate('', {
      format: 'T20',
      team1: 'IND',
      team2: 'AUS',
      matchId: 789,
      cutoffUtcIso: '2024-10-30T14:00:00Z',
    });
    expect(res).toEqual(payload);
    expect(mockFetch).toHaveBeenCalledTimes(1);
    const calledUrl = mockFetch.mock.calls[0][0] as string;
    expect(calledUrl).toContain('use_ml=1');
    expect(calledUrl).toContain('match_id=789');
  });
});
