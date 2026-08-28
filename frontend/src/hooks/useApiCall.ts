import { useMemo } from 'react';
import { useAsync } from './useAsync';

export interface UseApiCallResult<T> {
  data: T | null;
  error: string | null;
  loading: boolean;
  refetch: () => Promise<void>;
}

/**
 * The zero-argument, string-error view of {@link useAsync}.
 *
 * It stays because a dozen call sites read `error` as a string and pass `refetch` to
 * `usePolling`, and rewriting those to consume a structured error is W1-2's job, not
 * this hook's. What it no longer does is own a second copy of the lifecycle: the
 * staleness guard and unmount safety useAsync added apply here too, which they did
 * not when this was 20 lines of its own `useState`.
 *
 * Reach for `useAsync` in new code — it keeps the `hint` the backend sent.
 */
export function useApiCall<T>(
  fetchFn: () => Promise<T>,
  options?: { defaultErrorMessage?: string },
): UseApiCallResult<T> {
  const { data, loading, error, run } = useAsync(fetchFn, {
    errorMessage: options?.defaultErrorMessage ?? 'Request failed',
  });

  return useMemo(
    () => ({
      data,
      loading,
      error: error ? error.message : null,
      refetch: async () => {
        await run();
      },
    }),
    [data, loading, error, run],
  );
}
