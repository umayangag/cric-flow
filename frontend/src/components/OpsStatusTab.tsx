import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { api } from '../api';
import OpsBadges from './OpsBadges';
import OpsMatrix from './OpsMatrix';
import OpsSuggestions from './OpsSuggestions';
import Stack from '@mui/material/Stack';
import Button from '@mui/material/Button';
import Typography from '@mui/material/Typography';
import Paper from '@mui/material/Paper';
import Grid from '@mui/material/Grid';
import StatusPill from './common/StatusPill';
import JsonCollapse from './common/JsonCollapse';

// Minimal, forward-compatible Ops Status contract.
// Structured to match current UI needs while staying permissive for new fields.
type ServicesStatus = {
  api_health?: boolean;
  api_readiness?: boolean;
  ml_health?: boolean;
};

type PrecomputeFormats = Record<string, { status?: 'ok' | 'stale' | 'missing' | string } | undefined>;

type ExportFile = { name?: string; exists?: boolean };
type ExportFormats = Record<string, { files?: ExportFile[] } | undefined>;

type ArtifactUnit = { exists?: boolean; loaded?: boolean };
type ArtifactFormats = Record<string, { batting?: ArtifactUnit; bowling?: ArtifactUnit } | undefined>;

export type OpsStatus = {
  timestamp: string;
  services?: ServicesStatus;
  db?: unknown;
  precompute?: { formats?: PrecomputeFormats };
  exports?: { formats?: ExportFormats };
  artifacts?: { formats?: ArtifactFormats };
  suggestions?: Array<{ reason: string; commands: string[] }>;
  // Allow additional forward-compatible fields
  [key: string]: unknown;
};

const REFRESH_MS = 15000;

const OpsStatusTab: React.FC = () => {
  const [data, setData] = useState<OpsStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState<boolean>(false);
  const timerRef = useRef<number | null>(null);

  const fetchStatus = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await api.opsStatus();
      setData(res);
    } catch (e: any) {
      setError(e?.message ?? 'Failed to fetch /ops/status');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    let cancelled = false;

    const schedule = () => {
      // Use recursive setTimeout to play nicer with fake timers in tests
      timerRef.current = window.setTimeout(async () => {
        if (cancelled) return;
        await fetchStatus();
        if (!cancelled) schedule();
      }, REFRESH_MS);
    };

    fetchStatus();
    schedule();

    return () => {
      cancelled = true;
      if (timerRef.current) {
        window.clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [fetchStatus]);

  const lastUpdated = useMemo(() => {
    if (!data?.timestamp) return '';
    try {
      const d = new Date(data.timestamp);
      return isNaN(d.getTime()) ? String(data.timestamp) : d.toLocaleString();
    } catch {
      return String(data.timestamp);
    }
  }, [data]);

  return (
    <Stack spacing={2}>
      <Stack direction="row" spacing={1} alignItems="center">
        <Button
          variant="contained"
          onClick={fetchStatus}
          disabled={loading}
          title="Refresh Ops Status"
          aria-label="Refresh Ops Status"
        >
          {loading ? 'Refreshing…' : 'Refresh'}
        </Button>
        <Typography variant="body2" sx={{ opacity: 0.8 }}>
          Auto-refresh: {REFRESH_MS / 1000}s{lastUpdated ? ` • Last updated: ${lastUpdated}` : ''}
        </Typography>
      </Stack>

      {error && (
        <Typography color="error" role="alert" aria-live="polite">
          Error: {error}
        </Typography>
      )}

      {!data && !error && (
        <Typography component="div" variant="body2" sx={{ opacity: 0.8 }}>
          <em>Loading Ops Status…</em>
        </Typography>
      )}

      {data && (
        <Stack spacing={2}>
          <Paper elevation={1} sx={{ p: 2 }}>
            <Typography variant="h6" gutterBottom>Services</Typography>
            <OpsBadges services={data.services} timestamp={data.timestamp} />
          </Paper>

          <Grid container spacing={2}>
            <Grid item xs={12} md={6}>
              <Paper elevation={1} sx={{ p: 2 }}>
                <Typography variant="h6" gutterBottom>Database</Typography>
                <StatusPill
                  state={data.services?.api_readiness ? 'ok' : 'error'}
                  label={data.services?.api_readiness ? 'DB Ready' : 'DB Not Ready'}
                />
                <JsonCollapse data={data.db} summary="Show database details" />
              </Paper>
            </Grid>
            <Grid item xs={12} md={6}>
              <Paper elevation={1} sx={{ p: 2 }}>
                <Typography variant="h6" gutterBottom>Migrations & Misc</Typography>
                <Typography variant="body2" sx={{ opacity: 0.8 }}>
                  Additional server checks (if any) will appear here.
                </Typography>
              </Paper>
            </Grid>
          </Grid>

          <Paper elevation={1} sx={{ p: 2 }}>
            <OpsMatrix type="precompute" title="Precompute" data={data.precompute} />
          </Paper>

          <Paper elevation={1} sx={{ p: 2 }}>
            <OpsMatrix type="exports" title="Exports" data={data.exports} />
          </Paper>

          <Paper elevation={1} sx={{ p: 2 }}>
            <OpsMatrix type="artifacts" title="Artifacts" data={data.artifacts} />
          </Paper>

          <Paper elevation={1} sx={{ p: 2 }}>
            <OpsSuggestions suggestions={data.suggestions} />
          </Paper>

          <JsonCollapse data={data} summary="Show raw JSON payload" />
        </Stack>
      )}
    </Stack>
  );
};

export default OpsStatusTab;
