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
});
