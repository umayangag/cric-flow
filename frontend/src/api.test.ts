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
});
