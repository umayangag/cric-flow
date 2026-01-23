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
import SimpleStatTiles from './common/SimpleStatTiles';
import SectionCard from './common/SectionCard';

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
  // New optional sections surfaced by backend as raw objects
  fielding?: unknown;
  weather?: unknown;
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

  const filterSuggestions = useCallback(
    (kind: 'db' | 'precompute' | 'exports' | 'artifacts' | 'fielding' | 'weather') => {
      const all = Array.isArray(data?.suggestions) ? data!.suggestions : [];
      const has = (text?: string) => (text || '').toLowerCase();
      switch (kind) {
        case 'db':
          return all.filter(
            (s) => has(s.reason).includes('database') || s.commands?.some((c) => c.includes('migrate') || c.includes('cricsheet'))
          );
        case 'precompute':
          return all.filter(
            (s) => has(s.reason).includes('precompute') || s.commands?.some((c) => c.includes('precompute'))
          );
        case 'exports':
          return all.filter(
            (s) => has(s.reason).includes('export') || s.commands?.some((c) => c.includes('export-dataset'))
          );
        case 'artifacts':
          return all.filter(
            (s) => has(s.reason).includes('artifact') || s.commands?.some((c) => c.includes('train-'))
          );
        case 'fielding':
          return all.filter((s) => has(s.reason).includes('fielding'));
        case 'weather':
          return all.filter((s) => has(s.reason).includes('weather'));
        default:
          return all;
      }
    },
    [data]
  );

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
          <SectionCard title="Services">
            <SimpleStatTiles
              size="md"
              items={[
                { label: 'API health', value: data.services?.api_health === true, state: data.services?.api_health ? 'ok' : 'error' },
                { label: 'API ready', value: data.services?.api_readiness === true, state: data.services?.api_readiness ? 'ok' : 'error' },
                { label: 'ML health', value: data.services?.ml_health === true, state: data.services?.ml_health ? 'ok' : 'error' },
              ]}
            />
            <OpsBadges services={data.services} timestamp={data.timestamp} />
          </SectionCard>

          <Grid container spacing={2} alignItems="stretch">
            <Grid item xs={12}>
              <SectionCard title="Database">
                <SimpleStatTiles
                  size="md"
                  items={(() => {
                    const counts: any = (data as any)?.db?.counts || {};
                    const ready = data.services?.api_readiness === true;
                    return [
                      { label: 'Ready', value: ready, state: ready ? 'ok' : 'error', title: ready ? 'DB reachable' : 'DB not reachable' },
                      { label: 'Players', value: typeof counts.players === 'number' ? counts.players : '—', state: 'neutral' },
                      { label: 'Matches', value: typeof counts.matches === 'number' ? counts.matches : '—', state: 'neutral' },
                      { label: 'Innings', value: typeof counts.innings === 'number' ? counts.innings : '—', state: 'neutral' },
                    ];
                  })()}
                />
                <StatusPill
                  state={data.services?.api_readiness ? 'ok' : 'error'}
                  label={data.services?.api_readiness ? 'DB Ready' : 'DB Not Ready'}
                />
                <Typography variant="body2">
                  Last match data import:{' '}
                  <strong>
                    {(() => {
                      const v = (data?.db as any)?.last_match_import_at;
                      if (!v) return 'unknown';
                      try {
                        const d = new Date(v);
                        return isNaN(d.getTime()) ? String(v) : d.toLocaleString();
                      } catch {
                        return String(v);
                      }
                    })()}
                  </strong>
                </Typography>
                <JsonCollapse data={data.db} summary="Show database details" />
                <OpsSuggestions suggestions={filterSuggestions('db')} />
              </SectionCard>
            </Grid>
          </Grid>

          <Grid container spacing={2} alignItems="stretch">
            <Grid item xs={12}>
              <SectionCard title="Precompute">
                <SimpleStatTiles
                  size="md"
                  items={(() => {
                    const fm: any = (data as any)?.precompute?.formats || {};
                    const formats = ['TEST', 'ODI', 'T20I', 'T20'];
                    let ok = 0, stale = 0, missing = 0;
                    formats.forEach((f) => {
                      const st = fm?.[f]?.status as string | undefined;
                      if (st === 'ok') ok++; else if (st === 'stale') stale++; else missing++;
                    });
                    return [
                      { label: 'OK', value: ok, state: ok > 0 ? 'ok' : 'neutral' },
                      { label: 'Stale', value: stale, state: stale > 0 ? 'error' : 'neutral' },
                      { label: 'Missing', value: missing, state: missing > 0 ? 'error' : 'neutral' },
                    ];
                  })()}
                />
                <OpsMatrix type="precompute" title="Precompute" data={data.precompute} />
                <OpsSuggestions suggestions={filterSuggestions('precompute')} />
              </SectionCard>
            </Grid>
          </Grid>

          <Grid container spacing={2} alignItems="stretch">
            <Grid item xs={12}>
              <SectionCard title="Exports">
                <SimpleStatTiles
                  size="md"
                  items={(() => {
                    const fm: any = (data as any)?.exports?.formats || {};
                    const formats = ['TEST', 'ODI', 'T20I', 'T20'];
                    let present = 0, missing = 0;
                    const hasAnyExists = (files: any): boolean => Array.isArray(files) && files.some((e: any) => !!(e && e.exists === true));
                    formats.forEach((f) => {
                      const files = fm?.[f]?.files ?? [];
                      if (hasAnyExists(files)) present++; else missing++;
                    });
                    return [
                      { label: 'Present', value: present, state: present > 0 ? 'ok' : 'neutral' },
                      { label: 'Missing', value: missing, state: missing > 0 ? 'error' : 'neutral' },
                    ];
                  })()}
                />
                <OpsMatrix type="exports" title="Exports" data={data.exports} />
                <OpsSuggestions suggestions={filterSuggestions('exports')} />
              </SectionCard>
            </Grid>
          </Grid>

          <Grid container spacing={2} alignItems="stretch">
            <Grid item xs={12}>
              <SectionCard title="Artifacts">
                <SimpleStatTiles
                  size="md"
                  items={(() => {
                    const fm: any = (data as any)?.artifacts?.formats || {};
                    const formats = ['TEST', 'ODI', 'T20I', 'T20'];
                    let complete = 0, missing = 0;
                    formats.forEach((f) => {
                      const bat = fm?.[f]?.batting || {};
                      const bowl = fm?.[f]?.bowling || {};
                      const ok = bat?.exists === true && bowl?.exists === true;
                      if (ok) complete++; else missing++;
                    });
                    return [
                      { label: 'Complete', value: complete, state: complete > 0 ? 'ok' : 'neutral' },
                      { label: 'Missing', value: missing, state: missing > 0 ? 'error' : 'neutral' },
                    ];
                  })()}
                />
                <OpsMatrix type="artifacts" title="Artifacts" data={data.artifacts} />
                <OpsSuggestions suggestions={filterSuggestions('artifacts')} />
              </SectionCard>
            </Grid>
          </Grid>

          <Grid container spacing={2} alignItems="stretch">
            <Grid item xs={12} md={6}>
              <SectionCard title="Fielding Data" subtitle={(
                <span>Summary of fielding data availability and stats.</span>
              )}>
                {(() => {
                  const f: any = (data as any)?.fielding || {};
                  const available = f?.available === true;
                  const rows = typeof f?.rows === 'number' ? f.rows : undefined;
                  return (
                    <SimpleStatTiles
                      items={[
                        { label: 'Available', value: available, state: available ? 'ok' : 'error', title: available ? 'data available' : 'no data' },
                        { label: 'Rows', value: rows ?? '—', state: 'neutral', title: rows != null ? `${rows} rows` : 'unknown' },
                      ]}
                    />
                  );
                })()}
                <JsonCollapse data={data.fielding} summary="Show fielding details" />
                <OpsSuggestions suggestions={filterSuggestions('fielding')} />
              </SectionCard>
            </Grid>
            <Grid item xs={12} md={6}>
              <SectionCard title="Weather Data" subtitle={(
                <span>Summary of weather data availability and stats.</span>
              )}>
                {(() => {
                  const w: any = (data as any)?.weather || {};
                  const available = w?.available === true;
                  const rows = typeof w?.rows === 'number' ? w.rows : undefined;
                  return (
                    <SimpleStatTiles
                      items={[
                        { label: 'Available', value: available, state: available ? 'ok' : 'error', title: available ? 'data available' : 'no data' },
                        { label: 'Rows', value: rows ?? '—', state: 'neutral', title: rows != null ? `${rows} rows` : 'unknown' },
                      ]}
                    />
                  );
                })()}
                <JsonCollapse data={data.weather} summary="Show weather details" />
                <OpsSuggestions suggestions={filterSuggestions('weather')} />
              </SectionCard>
            </Grid>
          </Grid>

          {/* Removed global suggestions block to keep suggestions within each section */}

          <JsonCollapse data={data} summary="Show raw JSON payload" />
        </Stack>
      )}
    </Stack>
    );
};

export default OpsStatusTab;
