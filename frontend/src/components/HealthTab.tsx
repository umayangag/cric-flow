import React, { useMemo, useState } from 'react';
import { api } from '../api';
import type { HealthResponse } from '../types';
import { readFreshness, servedFreshnessLabel, UNKNOWN_FRESHNESS } from '../utils/opsStatusHelpers';
import type { OpsFreshness } from '../utils/opsStatusHelpers';
import { Button, Divider, Grid, Paper, Stack, Typography } from '@mui/material';
import StatusPill from './common/StatusPill';
import JsonCollapse from './common/JsonCollapse';
import KeyValueList from './common/KeyValueList';
import { formatWhen } from '../utils/format';
import { usePolling } from '../hooks/usePolling';

const HEALTH_REFRESH_MS = Number(import.meta.env.VITE_HEALTH_REFRESH_MS ?? 60000) || 60000;

const HealthTab: React.FC = () => {
  const [mlData, setMlData] = useState<HealthResponse | null>(null);
  const [freshness, setFreshness] = useState<OpsFreshness>(UNKNOWN_FRESHNESS);
  const [apiHealth, setApiHealth] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [latencyApiMs, setLatencyApiMs] = useState<number | null>(null);
  const [latencyMlMs, setLatencyMlMs] = useState<number | null>(null);
  const [lastChecked, setLastChecked] = useState<string | null>(null);

  const load = async () => {
    try {
      setLoading(true);
      setError(null);
      const t0Api = performance.now();
      const apiPromise = api.apiHealth().then((res) => {
        setLatencyApiMs(Math.max(0, Math.round(performance.now() - t0Api)));
        return res;
      });
      const t0Ml = performance.now();
      const mlPromise = api.health().then((res) => {
        setLatencyMlMs(Math.max(0, Math.round(performance.now() - t0Ml)));
        return res;
      });

      // The Ratings line is not read off this tab's own health poll: freshness is one
      // object, assembled once by go-app, and every surface that shows it reads that one
      // (P2-1). An unreachable go-app leaves it `unknown`, which is what it is.
      const opsPromise = api.opsStatus();

      const [apiResp, mlResp, opsResp] = await Promise.allSettled([
        apiPromise,
        mlPromise,
        opsPromise,
      ]);
      if (apiResp.status === 'fulfilled') setApiHealth(apiResp.value.status);
      if (mlResp.status === 'fulfilled') setMlData(mlResp.value);
      setFreshness(
        opsResp.status === 'fulfilled' ? readFreshness(opsResp.value) : UNKNOWN_FRESHNESS,
      );
      if (apiResp.status === 'rejected' && mlResp.status === 'rejected') {
        throw new Error('Both API and ML health checks failed');
      }
      setLastChecked(new Date().toISOString());
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  // Initial load + periodic refresh while the tab is mounted.
  usePolling(load, HEALTH_REFRESH_MS, true);

  const apiState = useMemo(() => {
    if (!apiHealth) return 'pending' as const;
    return apiHealth.toLowerCase() === 'ok' ? 'ok' : 'error';
  }, [apiHealth]);

  const mlState = useMemo(() => {
    if (!mlData) return 'pending' as const;
    return mlData.status?.toLowerCase() === 'ok' ? 'ok' : 'warn';
  }, [mlData]);

  const lastCheckedLocal = useMemo(
    () => (lastChecked ? formatWhen(lastChecked) : ''),
    [lastChecked],
  );

  // What the ML service is actually serving (H-16, H-11). "Healthy" is about the
  // process; these three are about whether it can answer -- which run is loaded, how far
  // its ratings go, and whether that is recent enough that a live request is not refused.
  const runItems = useMemo(() => {
    const ratingsItem = { label: 'Ratings', value: servedFreshnessLabel(freshness.served) };
    if (!mlData) {
      return [{ label: 'Loaded run', value: '—' }, { label: 'Formats', value: '—' }, ratingsItem];
    }
    return [
      { label: 'Loaded run', value: mlData.run_id || (mlData.error ? 'refused' : 'none') },
      {
        label: 'Formats',
        value: mlData.loaded_xi_formats?.length ? mlData.loaded_xi_formats.join(', ') : 'None',
      },
      ratingsItem,
      ...(mlData.error ? [{ label: 'Refused', value: mlData.error }] : []),
    ];
  }, [mlData, freshness]);

  return (
    <Stack spacing={2}>
      <Stack direction="row" spacing={1} alignItems="center">
        <Button variant="contained" onClick={load} disabled={loading}>
          {loading ? 'Refreshing…' : 'Refresh'}
        </Button>
        {error && (
          <Typography color="error" role="alert" aria-live="polite">
            Error: {error}
          </Typography>
        )}
      </Stack>

      <Grid container spacing={2}>
        <Grid item xs={12} md={6}>
          <Paper elevation={1} sx={{ p: 2 }}>
            <Typography variant="h6" gutterBottom>
              Go API
            </Typography>
            <Stack direction="row" spacing={1} alignItems="center">
              <StatusPill
                state={apiState}
                label={
                  apiState === 'ok' ? 'Healthy' : apiState === 'pending' ? 'Checking…' : 'Unhealthy'
                }
              />
              <Typography variant="body2" sx={{ opacity: 0.8 }}>
                Endpoint: {import.meta.env.VITE_API_URL || 'http://localhost:8080'}/health
              </Typography>
            </Stack>
            <Divider sx={{ my: 1 }} />
            <KeyValueList
              items={[
                {
                  label: 'Latency',
                  value: latencyApiMs != null ? `${latencyApiMs} ms` : '—',
                },
                { label: 'Last checked', value: lastCheckedLocal || '—' },
              ]}
            />
          </Paper>
        </Grid>
        <Grid item xs={12} md={6}>
          <Paper elevation={1} sx={{ p: 2 }}>
            <Typography variant="h6" gutterBottom>
              ML Service
            </Typography>
            <Stack direction="row" spacing={1} alignItems="center">
              <StatusPill
                state={mlState}
                label={
                  mlState === 'ok' ? 'Healthy' : mlState === 'pending' ? 'Checking…' : 'Issues'
                }
              />
              <Typography variant="body2" sx={{ opacity: 0.8 }}>
                Endpoint: {import.meta.env.VITE_ML_SERVICE_URL || 'http://localhost:8000'}
                /health
              </Typography>
            </Stack>
            <Divider sx={{ my: 1 }} />
            <KeyValueList
              items={[
                {
                  label: 'Latency',
                  value: latencyMlMs != null ? `${latencyMlMs} ms` : '—',
                },
                { label: 'Last checked', value: lastCheckedLocal || '—' },
                { label: 'Models dir', value: mlData?.models_dir || '—' },
                ...runItems,
              ]}
            />
          </Paper>
        </Grid>
      </Grid>

      {(mlData || apiHealth) && (
        <JsonCollapse data={{ api: apiHealth, ml: mlData, freshness }} summary="Show raw JSON" />
      )}
    </Stack>
  );
};

export default HealthTab;
