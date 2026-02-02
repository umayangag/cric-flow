import { describe, it, expect, beforeEach, vi } from 'vitest';
import { api } from './api';

// Simple fetch mock helper
function mockFetchOnce(data: any, ok = true, status = 200) {
  (globalThis as any).fetch = vi.fn().mockResolvedValue({
    ok,
    status,
    statusText: ok ? 'OK' : 'Bad Request',
    json: async () => data,
    text: async () => JSON.stringify(data),
  });
}

describe('frontend api client (DB-backed)', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('seasonsNext returns next_season payload', async () => {
    mockFetchOnce({ next_season: 2023 });
    const resp = await api.seasonsNext('2022-12-31', 'T20');
    expect(resp.next_season).toBe(2023);
    expect((globalThis as any).fetch).toHaveBeenCalledTimes(1);
    const urlArg = (globalThis as any).fetch.mock.calls[0][0];
    expect(String(urlArg)).toContain('/seasons/next');
    expect(String(urlArg)).toContain('cutoff=2022-12-31');
    expect(String(urlArg)).toContain('format=T20');
  });

  it('listMatches returns array of matches', async () => {
    mockFetchOnce([
      { match_id: 1, date: '2023-01-02', format: 'T20', teams: ['A', 'B'] },
      { match_id: 2, date: '2023-01-03', teams: ['C', 'D'] },
    ]);
    const items = await api.listMatches(2023, '2022-12-31');
    expect(items.length).toBe(2);
    expect(items[0].match_id).toBe(1);
    expect(items[1].teams[1]).toBe('D');
  });

  it('getMatchSquads returns squads payload', async () => {
    mockFetchOnce({
      match_id: 123,
      date: '2023-01-07',
      teams: ['Team A', 'Team B'],
      squads: [
        {
          team_name: 'Team A',
          actual_win: 1,
          players: [
            {
              player_name: 'A1',
              runs_scored: 0,
              balls_faced: 0,
              fours_scored: 0,
              sixes_scored: 0,
              batting_position: 0,
              strike_rate: 0,
              runs_conceded: 0,
              deliveries: 0,
              wickets_taken: 0,
              econ: 0,
            },
          ],
        },
        {
          team_name: 'Team B',
          actual_win: 0,
          players: [
            {
              player_name: 'B1',
              runs_scored: 0,
              balls_faced: 0,
              fours_scored: 0,
              sixes_scored: 0,
              batting_position: 0,
              strike_rate: 0,
              runs_conceded: 0,
              deliveries: 0,
              wickets_taken: 0,
              econ: 0,
            },
          ],
        },
      ],
    });
    const resp = await api.getMatchSquads(123, '2023-01-05');
    expect(resp.match_id).toBe(123);
    expect(resp.squads.length).toBe(2);
    expect(resp.squads[0].team_name).toBe('Team A');
  });

  it('httpApi throws on non-OK response', async () => {
    (globalThis as any).fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      statusText: 'Bad Request',
      json: async () => ({ code: 'INVALID_PARAM' }),
      text: async () => '{"code":"INVALID_PARAM"}',
    });
    await expect(api.seasonsNext('bad-date')).rejects.toBeInstanceOf(Error);
  });
});
