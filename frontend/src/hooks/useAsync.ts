import { useCallback, useEffect, useRef, useState } from 'react';
import { ApiError, toApiError } from '../lib/apiError';

/** What every asynchronous call in this app is worth knowing about. */
export interface AsyncState<T> {
  /** The last successful result, or null before one arrives. */
  data: T | null;
  /** True while a call is in flight. */
  loading: boolean;
  /** The last failure, structured. Cleared when a call starts. */
  error: ApiError | null;
}

export interface UseAsyncResult<Args extends unknown[], T> extends AsyncState<T> {
  /**
   * Start a call. Resolves with the result, or null when it failed — the failure is
   * in `error`, so callers that only want to sequence work can ignore the return.
   */
  run: (...args: Args) => Promise<T | null>;
  /** Back to the initial state: no data, not loading, no error. */
  reset: () => void;
  /** Replace the data without a call, for the rare case that already has it. */
  setData: (data: T | null) => void;
}

export interface UseAsyncOptions<Args extends unknown[]> {
  /** What to say when a rejection carried no message of its own. */
  errorMessage?: string;
  /**
   * Arguments to call with once on mount. `[]` for a call that takes none.
   *
   * It is the argument list rather than a boolean so the types stay honest: a call
   * that needs a format cannot be auto-run without one.
   */
  runOnMount?: Args;
}

/**
 * The one async-state primitive: `{ data, loading, error, run, reset }`.
 *
 * Three hooks held 49 `useState` calls between them, most of them the same three
 * fields spelled slightly differently, each with its own idea of what an error is.
 * That is why error handling differed between tabs and why every new panel
 * re-implemented the same lifecycle.
 *
 * Two things it does that the hand-written versions did not:
 *
 *  - **Stale results are discarded.** Each call takes a sequence number and only the
 *    newest may write state. Type a format, change it, and the slower first response
 *    can no longer land on top of the second — a race every open-coded version had.
 *  - **Nothing is set after unmount**, so navigating away mid-request is not a
 *    React warning and a leak.
 */
export function useAsync<Args extends unknown[], T>(
  fn: (...args: Args) => Promise<T>,
  options?: UseAsyncOptions<Args>,
): UseAsyncResult<Args, T> {
  const [state, setState] = useState<AsyncState<T>>({ data: null, loading: false, error: null });

  const errorMessage = options?.errorMessage;
  // Held in refs so `run` stays stable: an unstable `run` is an effect that refires,
  // which is the loop this hook exists to stop callers from writing.
  const fnRef = useRef(fn);
  fnRef.current = fn;
  const errorMessageRef = useRef(errorMessage);
  errorMessageRef.current = errorMessage;

  const latestCall = useRef(0);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const run = useCallback(async (...args: Args): Promise<T | null> => {
    const call = ++latestCall.current;
    const isCurrent = () => mounted.current && latestCall.current === call;

    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const data = await fnRef.current(...args);
      if (!isCurrent()) return data;
      setState({ data, loading: false, error: null });
      return data;
    } catch (thrown) {
      if (!isCurrent()) return null;
      setState({ data: null, loading: false, error: toApiError(thrown, errorMessageRef.current) });
      return null;
    }
  }, []);

  const reset = useCallback(() => {
    // Bumping the sequence retires any call in flight, so a response that arrives
    // after a reset cannot repopulate what was just cleared.
    latestCall.current += 1;
    setState({ data: null, loading: false, error: null });
  }, []);

  const setData = useCallback((data: T | null) => {
    setState((prev) => ({ ...prev, data }));
  }, []);

  const runOnMount = options?.runOnMount;
  // Deliberately mount-only: runOnMount describes the first call, not a subscription.
  // A caller that needs to re-run on a changing input calls run() from its own effect,
  // where the dependency is visible.
  const runOnMountRef = useRef(runOnMount);
  useEffect(() => {
    if (!runOnMountRef.current) return;
    void run(...runOnMountRef.current);
  }, [run]);

  return { ...state, run, reset, setData };
}
