import React, { useMemo, useState } from 'react';
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
import { usePolling } from '../hooks/usePolling';

const MODEL_TYPES = ['batting', 'bowling', 'fielding', 'extras', 'win'] as const;
const HEALTH_REFRESH_MS = Number(import.meta.env.VITE_HEALTH_REFRESH_MS ?? 60000) || 60000;

const ARTIFACT_LABELS: Record<(typeof MODEL_TYPES)[number], string> = {
  batting: 'bat',
  bowling: 'bowl',
  fielding: 'field',
  extras: 'extras',
  win: 'win',
};

type ArtifactItem = { file: string; size_bytes?: number; modified?: number };

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

  const loadedFormatsItems = useMemo(() => {
    return MODEL_TYPES.map((t) => {
      const formats = mlData?.[`loaded_${t}_formats` as keyof HealthResponse] as
        string[] | undefined;
      const value = !mlData ? '—' : formats && formats.length > 0 ? formats.join(', ') : 'None';
      return { label: `Loaded ${t === 'win' ? 'win prediction' : t}`, value };
    });
  }, [mlData]);

  const artifactsCountValue = useMemo(() => {
    if (!mlData) return '—';
    const parts = MODEL_TYPES.map((t) => {
      const arr = mlData.artifacts?.[t];
      return Array.isArray(arr) && arr.length ? `${ARTIFACT_LABELS[t]}: ${arr.length}` : null;
    }).filter(Boolean);
    return parts.length ? (parts as string[]).join(', ') : 'None';
  }, [mlData]);

  const allArtifacts = useMemo(() => {
    if (!mlData) return [];
    return MODEL_TYPES.flatMap((t) => mlData.artifacts?.[t] || []) as ArtifactItem[];
  }, [mlData]);

  const totalSizeValue = useMemo(() => {
    if (!mlData) return '—';
    const total = allArtifacts.reduce((acc, it) => acc + (it.size_bytes ?? 0), 0);
    return total > 0 ? formatBytes(total) : '0 B';
  }, [mlData, allArtifacts]);

  const latestModifiedValue = useMemo(() => {
    if (!mlData) return '—';
    const timestamps = allArtifacts
      .map((it) => it.modified)
      .filter((n): n is number => n != null && isFinite(n));
    if (!timestamps.length) return 'N/A';
    const max = Math.max(...timestamps);
    const d = new Date(max * 1000);
    return isNaN(d.getTime()) ? '—' : d.toLocaleString();
  }, [mlData, allArtifacts]);

  const legacyValue = useMemo(() => {
    if (!mlData) return '—';
    const legacy = MODEL_TYPES.map(
      (t) => mlData?.[`legacy_${t}_available` as keyof HealthResponse] && t,
    )
      .filter(Boolean)
      .join(', ');
    return legacy || 'None';
  }, [mlData]);

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
                ...loadedFormatsItems,
                { label: 'Legacy (unified)', value: legacyValue },
                { label: 'Artifacts', value: artifactsCountValue },
                { label: 'Total size', value: totalSizeValue },
                { label: 'Latest modified', value: latestModifiedValue },
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
