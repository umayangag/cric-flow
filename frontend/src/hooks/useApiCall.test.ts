import { describe, it, expect, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useApiCall } from './useApiCall';

describe('useApiCall', () => {
  /**
   * The regression this guards: callers pass `refetch` as a `useEffect` dependency.
   * When `refetch` was rebuilt on every state change, the response to a fetch
   * re-fired the effect that started it, and the tab refetched without pause.
   */
  it('keeps refetch identity stable across a completed call', async () => {
    const fetchFn = vi.fn().mockResolvedValue('ok');
    const { result } = renderHook(() => useApiCall(fetchFn));
    const initialRefetch = result.current.refetch;

    await act(async () => {
      await result.current.refetch();
    });

    await waitFor(() => expect(result.current.data).toBe('ok'));
    expect(result.current.refetch).toBe(initialRefetch);
    expect(fetchFn).toHaveBeenCalledTimes(1);
  });

  it('keeps refetch identity stable across a failed call', async () => {
    const fetchFn = vi.fn().mockRejectedValue(new Error('boom'));
    const { result } = renderHook(() =>
      useApiCall(fetchFn, { defaultErrorMessage: 'Failed to fetch' }),
    );
    const initialRefetch = result.current.refetch;

    await act(async () => {
      await result.current.refetch();
    });

    await waitFor(() => expect(result.current.error).toBe('boom'));
    expect(result.current.refetch).toBe(initialRefetch);
  });

  it('reports the default error message when the rejection carries none', async () => {
    const fetchFn = vi.fn().mockRejectedValue(new Error(''));
    const { result } = renderHook(() =>
      useApiCall(fetchFn, { defaultErrorMessage: 'Failed to fetch model stats' }),
    );

    await act(async () => {
      await result.current.refetch();
    });

    await waitFor(() => expect(result.current.error).toBe('Failed to fetch model stats'));
  });
});
