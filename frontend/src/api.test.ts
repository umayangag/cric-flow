import { describe, it, expect, beforeEach, vi } from 'vitest';
import type { Mock } from 'vitest';
import { api } from './api';

describe('frontend api client (DB-backed)', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('httpApi throws on non-OK response', async () => {
    (globalThis as unknown as { fetch: Mock }).fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      statusText: 'Bad Request',
      json: async () => ({ code: 'INVALID_PARAM' }),
      text: async () => '{"code":"INVALID_PARAM"}',
    }) as unknown as Mock;
    // Use an existing API method to test error handling
    await expect(api.apiHealth()).rejects.toBeInstanceOf(Error);
  });

  it('includes X-API-Key header when stored in localStorage', async () => {
    const mockStorage: Record<string, string> = { cric_info_api_key: 'test-key' };
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => mockStorage[key] || null,
      setItem: (key: string, value: string) => {
        mockStorage[key] = value;
      },
      removeItem: (key: string) => {
        delete mockStorage[key];
      },
    });

    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ status: 'ok' }),
    });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock as unknown as Mock;

    await api.apiHealth();

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining(':8080/health'),
      expect.objectContaining({
        headers: expect.objectContaining({
          'X-API-Key': 'test-key',
        }),
      }),
    );
    vi.unstubAllGlobals();
  });

  it('getTeamSidesByFormat fetches with format query param and returns sides', async () => {
    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    });
    const sides = [
      { club_id: 43, name: 'India', gender: 'male', display_name: 'India (men)' },
      { club_id: 132, name: 'India', gender: 'female', display_name: 'India (women)' },
    ];
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => sides });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock as unknown as Mock;

    const teams = await api.getTeamSidesByFormat('T20I');

    expect(teams).toEqual(sides);
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/api/options/teams-by-format'),
      expect.any(Object),
    );
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain('format=T20I');
    vi.unstubAllGlobals();
  });

  it('getOpponentSides addresses the club by id, not by name', async () => {
    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    });
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => [] });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock as unknown as Mock;

    await api.getOpponentSides('ODI', 43);

    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain('/api/options/opponents');
    expect(url).toContain('format=ODI');
    expect(url).toContain('team_id=43');
    expect(url).not.toContain('team=');
    vi.unstubAllGlobals();
  });

  it('predictTeamSelection POSTs params and returns selection', async () => {
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {}, removeItem: () => {} });
    const payload = {
      team1: [
        {
          player_id: 1,
          player_name: 'A',
          runs: 20,
          runs_range: { p10: 4, p90: 51 },
          wickets: 0,
          runs_conceded: 0,
        },
      ],
      team2: [],
      team1_side: { club_id: 132, name: 'India', gender: 'female', display_name: 'India (women)' },
      team2_side: {
        club_id: 12,
        name: 'Australia',
        gender: 'female',
        display_name: 'Australia (women)',
      },
      selection: { objective: 'win', optimised: true },
      win_probability: { team1: 0.61, source: 'display', predicted_winner: 'India (women)' },
    };
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => payload });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock as unknown as Mock;

    const result = await api.predictTeamSelection({
      format: 'T20I',
      team1_id: 132,
      team2_id: 12,
      match_date: '2024-06-15',
    });

    expect(result).toEqual(payload);
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/api/predict/team-selection'),
      expect.objectContaining({
        method: 'POST',
        // The side is named by id: "India" would not say which of two teams to score (D-11).
        body: expect.stringContaining('"team1_id":132'),
      }),
    );
    vi.unstubAllGlobals();
  });

  // D-10: a Stop used to report success whatever happened to the training process. The
  // console now depends on being told which steps really stopped, and on a partial stop
  // arriving as one — so both shapes are read here rather than assumed.
  it('opsPipelineStop reports the training steps that were stopped', async () => {
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {}, removeItem: () => {} });
    const body = {
      status: 'cancelled',
      cancelled: 1,
      plan_stopped: false,
      training_stopped: ['retrain'],
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => JSON.stringify(body),
    });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock as unknown as Mock;

    const { status, data } = await api.opsPipelineStop();

    expect(status).toBe(200);
    expect(data.training_stopped).toEqual(['retrain']);
    expect(data.status).toBe('cancelled');
    vi.unstubAllGlobals();
  });

  it('opsPipelineStop surfaces a stop ml-service could not confirm', async () => {
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {}, removeItem: () => {} });
    const body = {
      status: 'partially_cancelled',
      cancelled: 1,
      error: 'cancelled this run, but ml-service could not confirm its training process stopped',
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 502,
      text: async () => JSON.stringify(body),
    });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock as unknown as Mock;

    const { status, data } = await api.opsPipelineStop();

    expect(status).toBe(502);
    expect(data.status).toBe('partially_cancelled');
    expect(data.error).toMatch(/could not confirm/);
    vi.unstubAllGlobals();
  });

  it('searchVenues returns empty array when query has fewer than 3 characters', async () => {
    const result = await api.searchVenues('ab');
    expect(result).toEqual([]);
  });

  it('searchVenues fetches when query has 3+ characters', async () => {
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {}, removeItem: () => {} });
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ["Lord's", 'MCG'],
    });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock as unknown as Mock;
    const result = await api.searchVenues('lord');
    expect(result).toEqual(["Lord's", 'MCG']);
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/api/options/venues'),
      expect.any(Object),
    );
    vi.unstubAllGlobals();
  });

  it('evaluationReport fetches the harness report from the backtest surface', async () => {
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {}, removeItem: () => {} });
    const payload = { formats: {}, serving_parity: { passed: true } };
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => payload });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock as unknown as Mock;

    const result = await api.evaluationReport();

    expect(result).toEqual(payload);
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain('/api/backtest/report');
    vi.unstubAllGlobals();
  });

  it('opsStatus and health go to their own routes', async () => {
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {}, removeItem: () => {} });
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({}) });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock as unknown as Mock;

    await api.opsStatus();
    await api.health();

    expect(fetchMock.mock.calls[0][0]).toContain('/ops/status');
    expect(fetchMock.mock.calls[1][0]).toContain('/api/health/ml');
    vi.unstubAllGlobals();
  });
});
