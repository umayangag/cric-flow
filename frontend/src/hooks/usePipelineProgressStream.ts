import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../api';
import type { PipelineProgressPayload } from '../types';

const MAX_STREAM_RETRIES = 5;
const RETRY_DELAY_MS = 3000;
/** Keep showing the last payload for this long after a connection loss before erroring. */
const STALE_PROGRESS_BUFFER_MS = 15000;

/** What the panel needs to know about the live stream. */
export type PipelineProgressStream = {
  /** Latest payload, or null before the first event arrives. */
  payload: PipelineProgressPayload | null;
  /** Set once retries are exhausted or the buffer expires. */
  error: string | null;
  /** True while reconnecting but still showing the last known state. */
  reconnecting: boolean;
  /** True while waiting for the first event of a run. */
  connecting: boolean;
};

/**
 * Subscribes to GET /ops/pipeline/stream while a run is in flight.
 *
 * Connection handling lives here rather than in the panel so the panel is only about
 * what to draw: retry counting, the stale-progress grace period and the
 * run-completed notification are all one concern, and they are this one.
 */
export function usePipelineProgressStream(
  enabled: boolean,
  onRunCompleted?: (payload: PipelineProgressPayload) => void,
): PipelineProgressStream {
  const [payload, setPayload] = useState<PipelineProgressPayload | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reconnecting, setReconnecting] = useState(false);
  const [retryCount, setRetryCount] = useState(0);

  const wasRunningRef = useRef(false);
  const staleTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const onRunCompletedRef = useRef(onRunCompleted);
  onRunCompletedRef.current = onRunCompleted;

  const clearStaleTimer = useCallback(() => {
    if (staleTimerRef.current) {
      clearTimeout(staleTimerRef.current);
      staleTimerRef.current = null;
    }
  }, []);

  useEffect(() => {
    if (!enabled) {
      clearStaleTimer();
      setError(null);
      setReconnecting(false);
      setRetryCount(0);
      setPayload((prev) => (prev?.running ? { ...prev, running: false, steps: [] } : prev));
      return;
    }

    setError(null);
    setReconnecting(false);
    const ac = new AbortController();
    let mounted = true;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;

    const handleProgress = (p: PipelineProgressPayload) => {
      if (!mounted) return;
      clearStaleTimer();
      setError(null);
      setReconnecting(false);
      setPayload(p);
      if (p.running) {
        wasRunningRef.current = true;
        return;
      }
      if (wasRunningRef.current) {
        wasRunningRef.current = false;
        onRunCompletedRef.current?.(p);
      }
    };

    const handleFailure = (e: unknown) => {
      if (!mounted || (e as { name?: string }).name === 'AbortError') return;
      const message = e instanceof Error ? e.message : String(e);
      const isLastRetry = retryCount >= MAX_STREAM_RETRIES - 1;
      setReconnecting(true);
      clearStaleTimer();
      staleTimerRef.current = setTimeout(() => {
        staleTimerRef.current = null;
        if (!mounted) return;
        setReconnecting(false);
        setError(
          isLastRetry
            ? `${message}. Click Refresh to try again.`
            : 'Connection lost. Reconnecting…',
        );
      }, STALE_PROGRESS_BUFFER_MS);
      if (!isLastRetry) {
        retryTimer = setTimeout(() => setRetryCount((c) => c + 1), RETRY_DELAY_MS);
      }
    };

    api
      .subscribePipelineProgress(ac.signal, handleProgress)
      .then(() => {
        if (mounted && wasRunningRef.current) {
          wasRunningRef.current = false;
          onRunCompletedRef.current?.({ running: false, steps: [] });
        }
      })
      .catch(handleFailure);

    return () => {
      mounted = false;
      if (retryTimer) clearTimeout(retryTimer);
      clearStaleTimer();
      ac.abort();
    };
  }, [enabled, retryCount, clearStaleTimer]);

  return {
    payload,
    error,
    reconnecting,
    connecting: enabled && payload == null && error == null,
  };
}
