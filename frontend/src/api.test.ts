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

  it('getTeamsByFormat fetches with format query param', async () => {
    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    });
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ['IND', 'AUS', 'ENG'],
    });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock as unknown as Mock;

    const teams = await api.getTeamsByFormat('T20');

    expect(teams).toEqual(['IND', 'AUS', 'ENG']);
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/api/options/teams-by-format'),
      expect.any(Object),
    );
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain('format=T20');
    vi.unstubAllGlobals();
  });

  it('getOpponents fetches with format and team query params', async () => {
    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    });
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ['AUS', 'ENG', 'PAK'],
    });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock as unknown as Mock;

    const opponents = await api.getOpponents('ODI', 'IND');

    expect(opponents).toEqual(['AUS', 'ENG', 'PAK']);
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/api/options/opponents'),
      expect.any(Object),
    );
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain('format=ODI');
    expect(url).toContain('team=IND');
    vi.unstubAllGlobals();
  });

  it('accuracyTrend fetches with optional filters', async () => {
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {}, removeItem: () => {} });
    const payload = { points: [], metrics: [] };
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => payload });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock as unknown as Mock;

    await api.accuracyTrend({
      format: 'T20',
      start_date: '2024-01-01',
      end_date: '2024-12-31',
      team1: 'IND',
      team2: 'AUS',
      order: 'asc',
      limit: 50,
      cache: 'read',
      metrics: 'runs_mae',
    });

    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain('/api/backtest/accuracy-trend');
    expect(url).toContain('format=T20');
    expect(url).toContain('start_date=2024-01-01');
    expect(url).toContain('end_date=2024-12-31');
    expect(url).toContain('team1=IND');
    expect(url).toContain('team2=AUS');
    expect(url).toContain('order=asc');
    expect(url).toContain('limit=50');
    expect(url).toContain('cache=read');
    expect(url).toContain('metrics=runs_mae');
    expect(url).not.toContain('use_unified_model');
    vi.unstubAllGlobals();
  });

  it('evaluateStart POSTs and returns job_id', async () => {
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {}, removeItem: () => {} });
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ job_id: 'job-123' }),
    });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock as unknown as Mock;

    const result = await api.evaluateStart('T20', 'IND', 'AUS', 789, {
      use_latest_model: true,
    });

    expect(result).toEqual({ job_id: 'job-123' });
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/api/backtest/evaluate-start'),
      expect.objectContaining({ method: 'POST' }),
    );
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain('match_id=789');
    expect(url).not.toContain('use_unified_model');
    expect(url).toContain('use_latest_model=1');
    vi.unstubAllGlobals();
  });

  it('getEvaluateStatus fetches with job_id', async () => {
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {}, removeItem: () => {} });
    const payload = { status: 'done', result: {} };
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => payload });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock as unknown as Mock;

    const result = await api.getEvaluateStatus('job-456');
    expect(result).toEqual(payload);
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain('/api/backtest/evaluate-status');
    expect(url).toContain('job_id=job-456');
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
          wickets: 0,
          economy: 7,
          catches: 0,
          run_outs: 0,
        },
      ],
      team2: [],
    };
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => payload });
    (globalThis as unknown as { fetch: Mock }).fetch = fetchMock as unknown as Mock;

    const result = await api.predictTeamSelection({
      format: 'T20',
      team1: 'IND',
      team2: 'AUS',
      match_date: '2024-06-15',
      simulate: true,
      simulation_top_k: 3,
    });

    expect(result).toEqual(payload);
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/api/predict/team-selection'),
      expect.objectContaining({
        method: 'POST',
        body: expect.stringContaining('"format":"T20"'),
      }),
    );
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
});
