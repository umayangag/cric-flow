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
});
