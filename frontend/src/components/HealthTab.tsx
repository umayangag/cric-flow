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

const HealthTab: React.FC = () => {
  const [mlData, setMlData] = useState<HealthResponse | null>(null);
  const [apiHealth, setApiHealth] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const load = async () => {
    try {
      setLoading(true);
      setError(null);
      const [apiResp, mlResp] = await Promise.allSettled([
        api.apiHealth(),
        api.health(),
      ]);
      if (apiResp.status === 'fulfilled') setApiHealth(apiResp.value.status);
      if (mlResp.status === 'fulfilled') setMlData(mlResp.value);
      if (apiResp.status === 'rejected' && mlResp.status === 'rejected') {
        throw new Error('Both API and ML health checks failed');
      }
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
              <StatusPill state={apiState} label={apiState === 'ok' ? 'Healthy' : apiState === 'pending' ? 'Checking…' : 'Unhealthy'} />
              <Typography variant="body2" sx={{ opacity: 0.8 }}>
                Endpoint: {import.meta.env.VITE_API_URL || 'http://localhost:8080'}/health
              </Typography>
            </Stack>
          </Paper>
        </Grid>
        <Grid item xs={12} md={6}>
          <Paper elevation={1} sx={{ p: 2 }}>
            <Typography variant="h6" gutterBottom>
              ML Service
            </Typography>
            <Stack direction="row" spacing={1} alignItems="center">
              <StatusPill state={mlState} label={mlState === 'ok' ? 'Healthy' : mlState === 'pending' ? 'Checking…' : 'Issues'} />
              <Typography variant="body2" sx={{ opacity: 0.8 }}>
                Endpoint: {import.meta.env.VITE_ML_SERVICE_URL || 'http://localhost:8000'}/health
              </Typography>
            </Stack>
            {mlData && (
              <>
                <Divider sx={{ my: 1 }} />
                <Typography variant="body2" sx={{ opacity: 0.8 }}>
                  Formats loaded — Batting: {mlData.loaded_batting_formats?.join(', ') || '—'} | Bowling: {mlData.loaded_bowling_formats?.join(', ') || '—'}
                </Typography>
              </>
            )}
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
