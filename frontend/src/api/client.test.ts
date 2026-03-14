import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  toUpperTrim,
  isRFC3339,
  buildSelectQuery,
  buildEvaluateQuery,
  fetchBacktestSelect,
  fetchBacktestEvaluate,
  fetchFormats,
  fetchTeamsByFormat,
  fetchOpponents,
  fetchOpsMigrations,
  fetchOpsSuggestions,
} from './client';

// Note: use vi.stubGlobal to mock fetch to avoid duplicate global declarations

describe('api/client helpers', () => {
  it('toUpperTrim trims and uppercases', () => {
    expect(toUpperTrim('  ind  ')).toBe('IND');
    expect(toUpperTrim('')).toBe('');
    expect(toUpperTrim(null as unknown as string)).toBe('');
  });

  it('isRFC3339 accepts valid RFC3339 strings', () => {
    expect(isRFC3339('2024-10-30T14:00:00Z')).toBe(true);
    expect(isRFC3339('2024-10-30T14:00:00.123Z')).toBe(true);
    expect(isRFC3339('2024-10-30T14:00:00+05:30')).toBe(true);
  });

  it('isRFC3339 rejects invalid strings', () => {
    expect(isRFC3339('2024-10-30')).toBe(false);
    expect(isRFC3339('')).toBe(false);
    expect(isRFC3339('not-a-date')).toBe(false);
  });
});

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

  it('buildSelectQuery throws when format or teams missing', () => {
    expect(() => buildSelectQuery({ format: '', team1: 'IND', team2: 'AUS' })).toThrow(/required/);
    expect(() => buildSelectQuery({ format: 'T20', team1: '', team2: 'AUS' })).toThrow(/required/);
    expect(() => buildSelectQuery({ format: 'T20', team1: 'IND', team2: '' })).toThrow(/required/);
  });

  it('buildEvaluateQuery with useML false does not require cutoff', () => {
    const url = buildEvaluateQuery({
      format: 't20',
      team1: 'ind',
      team2: 'aus',
      matchId: 1,
      cutoffUtcIso: '',
      useML: false,
    });
    expect(url).toContain('mode=evaluate');
    expect(url).not.toContain('use_ml');
  });
});

describe('api/client fetch helpers', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('fetchBacktestSelect fetches and parses JSON', async () => {
    const payload = {
      filters: {},
      candidates: [
        {
          match_id: 1,
          stable_id: '',
          date: '',
          venue: '',
          season: '',
          format: 'T20',
          team1: 'IND',
          team2: 'AUS',
          winner_team_code: '',
        },
      ],
    };
    const mockFetch = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(payload) });
    vi.stubGlobal('fetch', mockFetch);
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
    vi.stubGlobal('fetch', mockFetch);
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

  it('fetchFormats fetches and parses JSON', async () => {
    const payload = ['T20', 'ODI', 'TEST'];
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(payload),
    });
    vi.stubGlobal('fetch', mockFetch);
    const res = await fetchFormats('http://localhost:8080');
    expect(res).toEqual(payload);
    expect(mockFetch).toHaveBeenCalledWith('http://localhost:8080/api/options/formats', undefined);
    vi.unstubAllGlobals();
  });

  it('fetchTeamsByFormat fetches with format param', async () => {
    const payload = ['IND', 'AUS', 'ENG'];
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(payload),
    });
    vi.stubGlobal('fetch', mockFetch);
    const res = await fetchTeamsByFormat('http://localhost:8080', 'T20');
    expect(res).toEqual(payload);
    const calledUrl = mockFetch.mock.calls[0][0] as string;
    expect(calledUrl).toContain('/api/options/teams-by-format');
    expect(calledUrl).toContain('format=T20');
    vi.unstubAllGlobals();
  });

  it('fetchOpponents fetches with format and team params', async () => {
    const payload = ['AUS', 'ENG'];
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(payload),
    });
    vi.stubGlobal('fetch', mockFetch);
    const res = await fetchOpponents('http://localhost:8080', 'ODI', 'IND');
    expect(res).toEqual(payload);
    const calledUrl = mockFetch.mock.calls[0][0] as string;
    expect(calledUrl).toContain('/api/options/opponents');
    expect(calledUrl).toContain('format=ODI');
    expect(calledUrl).toContain('team=IND');
    vi.unstubAllGlobals();
  });

  it('fetchBacktestSelect throws on non-OK response', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      text: () => Promise.resolve('server error'),
    });
    vi.stubGlobal('fetch', mockFetch);
    await expect(
      fetchBacktestSelect('http://localhost:8080', { format: 'T20', team1: 'IND', team2: 'AUS' }),
    ).rejects.toThrow(/HTTP 500/);
    vi.unstubAllGlobals();
  });

  it('fetchBacktestEvaluate throws on non-OK response', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      statusText: 'Not Found',
      text: () => Promise.resolve('not found'),
    });
    vi.stubGlobal('fetch', mockFetch);
    await expect(
      fetchBacktestEvaluate('http://localhost:8080', {
        format: 'T20',
        team1: 'IND',
        team2: 'AUS',
        matchId: 1,
        cutoffUtcIso: '2024-10-30T14:00:00Z',
      }),
    ).rejects.toThrow(/HTTP 404/);
    vi.unstubAllGlobals();
  });

  it('fetchOpsMigrations fetches with page and limit', async () => {
    const payload = { items: [], total: 0 };
    const mockFetch = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(payload) });
    vi.stubGlobal('fetch', mockFetch);
    const res = await fetchOpsMigrations('http://localhost:8080', 2, 20);
    expect(res).toEqual(payload);
    const calledUrl = mockFetch.mock.calls[0][0] as string;
    expect(calledUrl).toContain('page=2');
    expect(calledUrl).toContain('limit=20');
    vi.unstubAllGlobals();
  });

  it('fetchOpsSuggestions fetches and parses JSON', async () => {
    const payload = [{ id: '1', message: 'Run import' }];
    const mockFetch = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(payload) });
    vi.stubGlobal('fetch', mockFetch);
    const res = await fetchOpsSuggestions('http://localhost:8080');
    expect(res).toEqual(payload);
    expect(mockFetch).toHaveBeenCalledWith('http://localhost:8080/ops/suggestions');
    vi.unstubAllGlobals();
  });

  it('fetchFormats with relative baseUrl concatenates path', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(['T20']),
    });
    vi.stubGlobal('fetch', mockFetch);
    const res = await fetchFormats('');
    expect(res).toEqual(['T20']);
    expect(mockFetch).toHaveBeenCalledWith('/api/options/formats', undefined);
    vi.unstubAllGlobals();
  });
});
