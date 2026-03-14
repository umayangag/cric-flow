import { useCallback, useRef, useState } from 'react';

export interface UseApiCallResult<T> {
  data: T | null;
  error: string | null;
  loading: boolean;
  refetch: () => Promise<void>;
}

/**
 * Encapsulates the common pattern: call an async API, track loading/error/data, expose refetch.
 * Use for one-off or manually-triggered fetches; pair with usePolling when auto-refresh is needed.
 * refetch is stable so it can be used in useEffect or usePolling without extra dependencies.
 */
export function useApiCall<T>(
  fetchFn: () => Promise<T>,
  options?: { defaultErrorMessage?: string },
): UseApiCallResult<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const defaultMessage = options?.defaultErrorMessage ?? 'Request failed';
  const fetchFnRef = useRef(fetchFn);
  fetchFnRef.current = fetchFn;

  const refetch = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const result = await fetchFnRef.current();
      setData(result);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : defaultMessage);
    } finally {
      setLoading(false);
    }
  }, [defaultMessage]);

  return { data, error, loading, refetch };
}
