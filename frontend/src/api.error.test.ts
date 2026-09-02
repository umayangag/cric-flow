import { describe, it, expect, vi, afterEach } from 'vitest';
import { api } from './api';
import { ApiError } from './lib/apiError';

/**
 * These pin the half of W1-2 that lives in the client: the backend's structured error
 * has to survive the fetch. It previously did not — `httpApi` threw
 * `new Error('HTTP 404 Not Found: {…}')`, so `code`, `hint` and `available` existed
 * only as text inside a message no component could take apart.
 */
describe('api error parsing', () => {
  afterEach(() => vi.unstubAllGlobals());

  function respondWith(status: number, body: string) {
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {}, removeItem: () => {} });
    (globalThis as unknown as { fetch: unknown }).fetch = vi.fn().mockResolvedValue({
      ok: false,
      status,
      statusText: 'Not Found',
      text: async () => body,
    });
  }

  it('keeps code, hint and available from a go-app error', async () => {
    respondWith(
      404,
      JSON.stringify({
        code: 'MODEL_NOT_LOADED',
        message: 'Model for format T20I not loaded',
        hint: 'Train artifacts for this format.',
        available: ['ODI', 'TEST'],
      }),
    );

    const err = await api.xiStatus().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiError = err as ApiError;
    expect(apiError.status).toBe(404);
    expect(apiError.code).toBe('MODEL_NOT_LOADED');
    expect(apiError.message).toBe('Model for format T20I not loaded');
    expect(apiError.hint).toBe('Train artifacts for this format.');
    expect(apiError.available).toEqual(['ODI', 'TEST']);
  });

  it('unwraps ml-service’s {detail: …} envelope', async () => {
    respondWith(
      400,
      JSON.stringify({ detail: { code: 'MISSING_FORMAT', message: "Missing 'format'" } }),
    );

    const err = (await api.xiStatus().catch((e: unknown) => e)) as ApiError;
    expect(err.code).toBe('MISSING_FORMAT');
    expect(err.message).toBe("Missing 'format'");
  });

  it('falls back to the raw body when the response is not JSON', async () => {
    respondWith(502, '<html>upstream is down</html>');

    const err = (await api.xiStatus().catch((e: unknown) => e)) as ApiError;
    expect(err.message).toContain('upstream is down');
    expect(err.status).toBe(502);
  });
});
