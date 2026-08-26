import { useCallback, useEffect, useState } from 'react';
import { api } from '../api';
import type { DataFeedsResponse, DatasetRegistryResponse, StagedResponse } from '../types';

/** Poll interval while a data step is running; slower once idle. */
const ACTIVE_REFRESH_MS = 4000;

/** Everything the Data tab needs, and whether a data step is in flight. */
export type DataTabState = {
  feeds?: DataFeedsResponse;
  staged?: StagedResponse;
  registry?: DatasetRegistryResponse;
  /** True while fetch or extract is running — both are in the same lane, so only one can be. */
  busy: boolean;
  error: string | null;
  loading: boolean;
  refresh: () => void;
  /** Call after starting a job so the tab shows it without waiting for a poll. */
  markStarted: () => void;
};

/**
 * Loads the Data tab's three views and tracks whether a dataset step is running.
 *
 * `busy` is asked of /ops/status rather than inferred from "we just clicked fetch":
 * a step may have been started from another browser tab, or be left over from a
 * previous session, and a button that looks available but 409s is worse than one that
 * says why it is disabled.
 *
 * Feeds are fetched once — the allowlist and the feed list do not change while the
 * page is open. Staged archives and the registry are refetched on demand and while a
 * step runs, because both change as a result of one.
 */
export function useDataTab(): DataTabState {
  const [feeds, setFeeds] = useState<DataFeedsResponse | undefined>();
  const [staged, setStaged] = useState<StagedResponse | undefined>();
  const [registry, setRegistry] = useState<DatasetRegistryResponse | undefined>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [tick, setTick] = useState(0);

  const refresh = useCallback(() => setTick((t) => t + 1), []);

  const markStarted = useCallback(() => {
    // Assume busy immediately: the job is started but /ops/status may not show it
    // until its row lands, and re-enabling the button in that window invites a 409.
    setBusy(true);
    refresh();
  }, [refresh]);

  useEffect(() => {
    const ac = new AbortController();
    let cancelled = false;

    const load = async () => {
      try {
        const [feedsRes, stagedRes, registryRes, status] = await Promise.all([
          feeds ? Promise.resolve(feeds) : api.opsDataFeeds({ signal: ac.signal }),
          api.opsDataStaged({ signal: ac.signal }),
          api.opsDatasets(50, { signal: ac.signal }),
          api.opsStatus(),
        ]);
        if (cancelled) return;
        setFeeds(feedsRes);
        setStaged(stagedRes);
        setRegistry(registryRes);
        // Both data steps share a lane, so either running means the lane is taken.
        const steps = status?.pipeline?.steps ?? {};
        setBusy(Boolean(steps.fetch?.running) || Boolean(steps.extract?.running));
        setError(null);
      } catch (e) {
        if (cancelled || (e as { name?: string }).name === 'AbortError') return;
        setError(e instanceof Error ? e.message : 'Failed to load dataset state');
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    void load();
    return () => {
      cancelled = true;
      ac.abort();
    };
    // feeds is intentionally not a dependency: including it would refetch everything
    // the moment feeds arrive, which is a wasted round trip on every mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tick]);

  // While a step runs, keep staged/registry/busy fresh so the tab settles by itself
  // when it finishes rather than waiting for the operator to click Refresh.
  useEffect(() => {
    if (!busy) return;
    const id = setInterval(refresh, ACTIVE_REFRESH_MS);
    return () => clearInterval(id);
  }, [busy, refresh]);

  return { feeds, staged, registry, busy, error, loading, refresh, markStarted };
}
