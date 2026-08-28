import { useEffect, useRef, useState } from 'react';

type PollingCallback = () => void | Promise<void>;

/** Reports whether the page is currently visible, updating as the tab is switched. */
function usePageVisible(): boolean {
  const [visible, setVisible] = useState(
    () => typeof document === 'undefined' || document.visibilityState !== 'hidden',
  );

  useEffect(() => {
    if (typeof document === 'undefined') return;
    const onChange = () => setVisible(document.visibilityState !== 'hidden');
    document.addEventListener('visibilitychange', onChange);
    return () => document.removeEventListener('visibilitychange', onChange);
  }, []);

  return visible;
}

/**
 * usePolling runs the provided callback immediately and then on a fixed interval
 * while `enabled` is true. It uses setTimeout recursively instead of setInterval
 * so that each run completes before the next is scheduled.
 *
 * **Polling stops while the tab is hidden** and resumes — with an immediate run — when
 * it comes back. A background tab left open overnight was otherwise asking the server
 * a question every two seconds that nobody was there to read the answer to, and the
 * first thing a returning user wants is fresh state anyway, which the resume gives
 * them without waiting out an interval.
 */
export function usePolling(callback: PollingCallback, intervalMs: number, enabled: boolean): void {
  const callbackRef = useRef<PollingCallback>(callback);
  const visible = usePageVisible();

  // Always keep latest callback without resubscribing the effect.
  useEffect(() => {
    callbackRef.current = callback;
  }, [callback]);

  useEffect(() => {
    if (!enabled || !visible || intervalMs <= 0) return;

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
  }, [enabled, intervalMs, visible]);
}
