import { useEffect, useRef } from 'react';

type PollingCallback = () => void | Promise<void>;

/**
 * usePolling runs the provided callback immediately and then on a fixed interval
 * while `enabled` is true. It uses setTimeout recursively instead of setInterval
 * so that each run completes before the next is scheduled.
 */
export function usePolling(callback: PollingCallback, intervalMs: number, enabled: boolean): void {
  const callbackRef = useRef<PollingCallback>(callback);

  // Always keep latest callback without resubscribing the effect.
  useEffect(() => {
    callbackRef.current = callback;
  }, [callback]);

  useEffect(() => {
    if (!enabled || intervalMs <= 0) return;

    let cancelled = false;
    let timeoutId: number | null = null;

    const tick = async () => {
      if (cancelled) return;
      try {
        await callbackRef.current();
      } finally {
        if (!cancelled) {
          timeoutId = window.setTimeout(tick, intervalMs);
        }
      }
    };

    // Run once immediately, then schedule subsequent runs.
    void tick();

    return () => {
      cancelled = true;
      if (timeoutId !== null) {
        window.clearTimeout(timeoutId);
        timeoutId = null;
      }
    };
  }, [enabled, intervalMs]);
}

