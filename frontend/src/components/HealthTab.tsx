import React, { useEffect, useMemo, useState } from 'react';
import { api } from '../api';
import type { HealthResponse } from '../types';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Button from '@mui/material/Button';
import Typography from '@mui/material/Typography';
import Grid from '@mui/material/Grid';
import Divider from '@mui/material/Divider';
import StatusPill from './common/StatusPill';
import JsonCollapse from './common/JsonCollapse';
import KeyValueList from './common/KeyValueList';

const HealthTab: React.FC = () => {
  const [mlData, setMlData] = useState<HealthResponse | null>(null);
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

      const [apiResp, mlResp] = await Promise.allSettled([apiPromise, mlPromise]);
      if (apiResp.status === 'fulfilled') setApiHealth(apiResp.value.status);
      if (mlResp.status === 'fulfilled') setMlData(mlResp.value);
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

  useEffect(() => {
    load();
  }, []);

  const apiState = useMemo(() => {
    if (!apiHealth) return 'pending' as const;
    return apiHealth.toLowerCase() === 'ok' ? 'ok' : 'error';
  }, [apiHealth]);

  const mlState = useMemo(() => {
    if (!mlData) return 'pending' as const;
    return mlData.status?.toLowerCase() === 'ok' ? 'ok' : 'warn';
  }, [mlData]);

  const formatBytes = (n: number): string => {
    if (!isFinite(n) || n <= 0) return '—';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;
    let v = n;
    while (v >= 1024 && i < units.length - 1) {
      v /= 1024;
      i++;
    }
    return `${v.toFixed(v < 10 && i > 0 ? 1 : 0)} ${units[i]}`;
  };

  const lastCheckedLocal = useMemo(() => {
    if (!lastChecked) return '';
    const d = new Date(lastChecked);
    return isNaN(d.getTime()) ? '' : d.toLocaleString();
  }, [lastChecked]);

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
                {
                  label: 'Loaded batting',
                  value:
                    (mlData?.loaded_batting_formats?.length ?? 0) > 0
                      ? mlData!.loaded_batting_formats!.join(', ')
                      : mlData
                        ? 'None'
                        : '—',
                },
                {
                  label: 'Loaded bowling',
                  value:
                    (mlData?.loaded_bowling_formats?.length ?? 0) > 0
                      ? mlData!.loaded_bowling_formats!.join(', ')
                      : mlData
                        ? 'None'
                        : '—',
                },
                { label: 'Models dir', value: mlData?.models_dir || '—' },
                {
                  label: 'Legacy batting',
                  value: mlData ? (mlData.legacy_batting_available ? 'Yes' : 'No') : '—',
                },
                {
                  label: 'Legacy bowling',
                  value: mlData ? (mlData.legacy_bowling_available ? 'Yes' : 'No') : '—',
                },
                (() => {
                  const bat = mlData?.artifacts?.batting || [];
                  const bowl = mlData?.artifacts?.bowling || [];
                  const batCount = Array.isArray(bat) ? bat.length : 0;
                  const bowlCount = Array.isArray(bowl) ? bowl.length : 0;
                  return {
                    label: 'Artifacts',
                    value: `batting: ${batCount}, bowling: ${bowlCount}`,
                  };
                })(),
                (() => {
                  const all = [
                    ...(mlData?.artifacts?.batting || []),
                    ...(mlData?.artifacts?.bowling || []),
                  ] as unknown[];
                  const total: number = all.reduce<number>((acc, it) => {
                    if (it && typeof it === 'object') {
                      const val = (it as Record<string, unknown>).size_bytes;
                      const n = typeof val === 'number' ? val : 0;
                      return acc + n;
                    }
                    return acc;
                  }, 0);
                  return {
                    label: 'Total size',
                    value: mlData ? (total > 0 ? formatBytes(total as number) : '0 B') : '—',
                  };
                })(),
                (() => {
                  const all = [
                    ...(mlData?.artifacts?.batting || []),
                    ...(mlData?.artifacts?.bowling || []),
                  ] as unknown[];
                  const latest = all
                    .map((it) => {
                      if (it && typeof it === 'object') {
                        const val = (it as Record<string, unknown>).modified;
                        return typeof val === 'number' ? val : NaN;
                      }
                      return NaN;
                    })
                    .filter((n): n is number => typeof n === 'number' && isFinite(n));
                  const max = latest.length ? Math.max(...latest) : NaN;
                  if (!isFinite(max))
                    return {
                      label: 'Latest modified',
                      value: mlData ? 'N/A' : '—',
                    };
                  const d = new Date(max * 1000);
                  return {
                    label: 'Latest modified',
                    value: isNaN(d.getTime()) ? '—' : d.toLocaleString(),
                  };
                })(),
              ]}
            />
          </Paper>
        </Grid>
      </Grid>

      {(mlData || apiHealth) && (
        <JsonCollapse data={{ api: apiHealth, ml: mlData }} summary="Show raw JSON" />
      )}
    </Stack>
  );
};

export default HealthTab;
