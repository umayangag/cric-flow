import { describe, it, expect, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useAsync } from './useAsync';
import { ApiError } from '../lib/apiError';

/** A promise plus the handles to settle it, so a test can control call ordering. */
function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe('useAsync', () => {
  it('runs on mount with the given arguments', async () => {
    const fn = vi.fn().mockResolvedValue('ok');
    const { result } = renderHook(() => useAsync(fn, { runOnMount: ['T20'] }));

    await waitFor(() => expect(result.current.data).toBe('ok'));
    expect(fn).toHaveBeenCalledWith('T20');
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it('does not run on mount when no arguments are given', () => {
    const fn = vi.fn().mockResolvedValue('ok');
    renderHook(() => useAsync(fn));
    expect(fn).not.toHaveBeenCalled();
  });

  /**
   * The race every hand-written version had: two calls in flight, the first slower.
   * Without a staleness guard the first response lands last and the UI shows the
   * answer to the question the user already changed.
   */
  it('ignores a slow earlier call that resolves after a later one', async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    const fn = vi
      .fn()
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce(() => second.promise);

    const { result } = renderHook(() => useAsync(fn));

    await act(async () => {
      void result.current.run();
      void result.current.run();
    });

    await act(async () => {
      second.resolve('newest');
      await second.promise;
    });
    await waitFor(() => expect(result.current.data).toBe('newest'));

    await act(async () => {
      first.resolve('stale');
      await first.promise;
    });
    expect(result.current.data).toBe('newest');
  });

  it('surfaces a failure as a structured ApiError and clears it on the next run', async () => {
    const fn = vi
      .fn()
      .mockRejectedValueOnce(
        new ApiError('no model', { code: 'MODEL_NOT_LOADED', hint: 'train it' }),
      )
      .mockResolvedValueOnce('ok');

    const { result } = renderHook(() => useAsync(fn));

    await act(async () => {
      await result.current.run();
    });
    expect(result.current.error?.code).toBe('MODEL_NOT_LOADED');
    expect(result.current.error?.hint).toBe('train it');

    await act(async () => {
      await result.current.run();
    });
    expect(result.current.error).toBeNull();
    expect(result.current.data).toBe('ok');
  });

  it('reset clears state and retires a call still in flight', async () => {
    const pending = deferred<string>();
    const fn = vi.fn().mockReturnValue(pending.promise);
    const { result } = renderHook(() => useAsync(fn));

    await act(async () => {
      void result.current.run();
    });
    act(() => result.current.reset());
    expect(result.current.loading).toBe(false);

    await act(async () => {
      pending.resolve('late');
      await pending.promise;
    });
    expect(result.current.data).toBeNull();
  });
});
