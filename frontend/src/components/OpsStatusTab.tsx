import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FormatHierarchyNode } from '../types';
import { api } from '../api';
import OpsMatrix from './OpsMatrix';
import OpsPipelineGraph from './OpsPipelineGraph';
import PipelineProgressPanel from './PipelineProgressPanel';
import MLPredictionGraph from './MLPredictionGraph';
import OpsMigrationsTable from './OpsMigrationsTable';
import OpsFormatHierarchy from './OpsFormatHierarchy';
import OpsTableStats from './OpsTableStats';
import Stack from '@mui/material/Stack';
import Button from '@mui/material/Button';
import Typography from '@mui/material/Typography';
import Grid from '@mui/material/Grid';
import StatusPill from './common/StatusPill';
import JsonCollapse from './common/JsonCollapse';
import SimpleStatTiles from './common/SimpleStatTiles';
import SectionCard from './common/SectionCard';
import { TableStat } from '../types';

// Local helpers for safely reading dynamic sections
const FORMATS = ['TEST', 'ODI', 'T20I', 'T20'] as const;
type FormatCode = (typeof FORMATS)[number];

function asObj(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' ? (v as Record<string, unknown>) : {};
}

function getFormats(section: unknown): Record<string, unknown> {
  const obj = asObj(section);
  return asObj((obj as { formats?: unknown }).formats);
}

function readStatus(v: unknown): 'ok' | 'stale' | 'missing' | 'unknown' {
  const s = typeof v === 'string' ? v : undefined;
  if (s === 'ok' || s === 'stale' || s === 'missing' || s === 'unknown') return s;
  return 'unknown';
}

function readNumber(v: unknown): number | undefined {
  if (typeof v === 'number' && isFinite(v)) return v;
  return undefined;
}

// Minimal, forward-compatible Ops Status contract.
// Structured to match current UI needs while staying permissive for new fields.
type ServicesStatus = {
  api_health?: boolean;
  api_readiness?: boolean;
  ml_health?: boolean;
};

type PrecomputeFormats = Record<
  string,
  { status?: 'ok' | 'stale' | 'missing' | string } | undefined
>;

type ExportFile = { name?: string; exists?: boolean };
type ExportFormats = Record<string, { files?: ExportFile[] } | undefined>;

type ArtifactUnit = { exists?: boolean; loaded?: boolean };
type ArtifactFormats = Record<
  string,
  { batting?: ArtifactUnit; bowling?: ArtifactUnit } | undefined
>;

export type OpsStatus = {
  timestamp: string;
  services?: ServicesStatus;
  db?: unknown;
  precompute?: { formats?: PrecomputeFormats };
  exports?: { formats?: ExportFormats };
  artifacts?: { formats?: ArtifactFormats };
  pipeline?: { steps?: Record<string, { running?: boolean }> };
  // New optional sections surfaced by backend as raw objects
  fielding?: unknown;
  weather?: unknown;
  hierarchy?: FormatHierarchyNode[];
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
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to fetch /ops/status');
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
          <SectionCard
            title="Pipeline"
            subtitle="Import → precompute → export → train (uses config + DB params) or auto-tune (discover params). Click a step to copy its command."
          >
            <OpsPipelineGraph data={data} onRefresh={fetchStatus} />
            <PipelineProgressPanel
              pipelineRunning={Object.values(asObj(data.pipeline?.steps ?? {})).some(
                (s) => asObj(s).running === true,
              )}
              onRefresh={fetchStatus}
            />
          </SectionCard>
          <SectionCard
            title="Prediction model flow"
            subtitle="Features at cutoff → per-player models (batting, bowling, fielding) → team aggregates + extras → win model (winner and team scores reconciled to win probability) → team selection and simulation."
          >
            <MLPredictionGraph />
          </SectionCard>
          <SectionCard title="Migration History">
            <OpsMigrationsTable />
          </SectionCard>

          <Grid container spacing={2} alignItems="stretch">
            <Grid item xs={12}>
              <SectionCard title="Database">
                <SimpleStatTiles
                  size="md"
                  items={(() => {
                    const dbObj = asObj(data.db);
                    const counts = asObj((dbObj as { counts?: unknown }).counts);
                    const ready = data.services?.api_readiness === true;
                    return [
                      {
                        label: 'Ready',
                        value: ready,
                        state: ready ? 'ok' : 'error',
                        title: ready ? 'DB reachable' : 'DB not reachable',
                      },
                      {
                        label: 'Players',
                        value: (() => {
                          const playersVal = (counts as Record<string, unknown>).players;
                          return typeof playersVal === 'number' ? playersVal : '—';
                        })(),
                        state: 'neutral',
                      },
                      {
                        label: 'Matches',
                        value: (() => {
                          const matchesVal = (counts as Record<string, unknown>).matches;
                          return typeof matchesVal === 'number' ? matchesVal : '—';
                        })(),
                        state: 'neutral',
                      },
                    ];
                  })()}
                />
                <StatusPill
                  state={data.services?.api_readiness ? 'ok' : 'error'}
                  label={data.services?.api_readiness ? 'DB Ready' : 'DB Not Ready'}
                />
                <Typography variant="body2" component="div">
                  Last match data import:{' '}
                  <strong>
                    {(() => {
                      const v = asObj(data?.db).last_match_import_at as unknown as
                        | string
                        | number
                        | Date
                        | undefined;
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
                {(() => {
                  const dbObj = asObj(data?.db);
                  const ts = Array.isArray((dbObj as { table_stats?: unknown }).table_stats)
                    ? ((dbObj as { table_stats?: unknown }).table_stats as unknown as TableStat[])
                    : undefined;
                  return <OpsTableStats stats={ts} />;
                })()}
                <JsonCollapse data={data.db} summary="Show database details" />
              </SectionCard>
            </Grid>
          </Grid>

          {/* Two blocks per row below Database */}
          <Grid container spacing={2} alignItems="stretch">
            <Grid item xs={12} md={6}>
              {(() => {
                const freshness = asObj((data as { db_freshness?: unknown })?.db_freshness);
                const overall = asObj(freshness.overall);
                const overallSt = readStatus(overall.status);
                const overallCount = readNumber(overall.match_count);
                const fm = getFormats(freshness);
                return (
                  <SectionCard
                    title="DB Data Freshness"
                    subtitle={
                      <Stack direction="row" spacing={1} alignItems="center">
                        <Typography variant="body2" component="div">
                          Overall
                        </Typography>
                        <StatusPill state={overallSt} label={overallSt} />
                        {overallCount != null && (
                          <Typography variant="body2" sx={{ opacity: 0.85 }}>
                            {overallCount.toLocaleString()} matches
                          </Typography>
                        )}
                      </Stack>
                    }
                  >
                    {Object.keys(fm).length === 0 ? (
                      <Typography variant="body2" sx={{ opacity: 0.7 }}>
                        Not available
                      </Typography>
                    ) : (
                      <Stack spacing={1}>
                        {FORMATS.map((f: FormatCode) => {
                          const row = asObj(fm[f]);
                          const st = readStatus(row.status);
                          const latest =
                            typeof row.latest_match_date === 'string'
                              ? row.latest_match_date
                              : undefined;
                          const days = readNumber(row.days_since);
                          const matchCount = readNumber(row.match_count);
                          return (
                            <Stack
                              key={f}
                              direction="row"
                              alignItems="center"
                              justifyContent="space-between"
                            >
                              <Stack direction="row" spacing={1} alignItems="center">
                                <Typography variant="body2" sx={{ minWidth: 48 }}>
                                  {f}
                                </Typography>
                                <StatusPill state={st} label={st} />
                              </Stack>
                              <Typography variant="body2" sx={{ opacity: 0.85 }}>
                                {matchCount != null && (
                                  <strong>{matchCount.toLocaleString()} matches</strong>
                                )}
                                {matchCount != null && (latest || st === 'missing') && ' · '}
                                {latest ? (
                                  <>
                                    latest {latest}
                                    {days != null ? ` · ${Math.max(0, Math.floor(days))}d ago` : ''}
                                  </>
                                ) : st === 'missing' ? (
                                  'no recent matches'
                                ) : (
                                  'not available'
                                )}
                              </Typography>
                            </Stack>
                          );
                        })}
                      </Stack>
                    )}
                  </SectionCard>
                );
              })()}
            </Grid>
            <Grid item xs={12} md={6}>
              {(() => {
                const comp = asObj((data as { db_completeness?: unknown })?.db_completeness);
                const overallObj = asObj(comp.overall);
                const overallSt = readStatus(overallObj.status);
                const overallLast30 = readNumber(overallObj.matches_last_30d);
                const fm = getFormats(comp);
                return (
                  <SectionCard
                    title="DB Data Completeness (last 30d)"
                    subtitle={
                      <Stack direction="row" spacing={1} alignItems="center">
                        <Typography variant="body2" component="div">
                          Overall
                        </Typography>
                        <StatusPill state={overallSt} label={overallSt} />
                        {overallLast30 != null && (
                          <Typography variant="body2" sx={{ opacity: 0.85 }}>
                            {overallLast30.toLocaleString()} matches in last 30d
                          </Typography>
                        )}
                      </Stack>
                    }
                  >
                    {Object.keys(fm).length === 0 ? (
                      <Typography variant="body2" sx={{ opacity: 0.7 }}>
                        Not available
                      </Typography>
                    ) : (
                      <Stack spacing={1}>
                        {FORMATS.map((f: FormatCode) => {
                          const row = asObj(fm[f]);
                          const st = readStatus(row.status);
                          const n = readNumber(row.matches_last_30d) ?? 0;
                          const min = readNumber(row.expected_min_30d) ?? 1;
                          return (
                            <Stack
                              key={f}
                              direction="row"
                              alignItems="center"
                              justifyContent="space-between"
                            >
                              <Stack direction="row" spacing={1} alignItems="center">
                                <Typography variant="body2" sx={{ minWidth: 48 }}>
                                  {f}
                                </Typography>
                                <StatusPill state={st} label={st} />
                              </Stack>
                              <Typography variant="body2" sx={{ opacity: 0.85 }}>
                                {`${n} of ${min} expected in last 30d`}
                              </Typography>
                            </Stack>
                          );
                        })}
                      </Stack>
                    )}
                  </SectionCard>
                );
              })()}
            </Grid>

            <Grid item xs={12} md={6}>
              <SectionCard
                title="Precompute"
                subtitle={(() => {
                  const pre = asObj(data.precompute);
                  const lastRun = pre.last_run as string | undefined;
                  const asOf = pre.as_of as string | undefined;
                  if (!lastRun && !asOf) return 'Unified (all formats). Stats per format below.';
                  return (
                    <Typography variant="body2" component="span" sx={{ opacity: 0.9 }}>
                      Unified (all formats). Last run:{' '}
                      {lastRun ? new Date(lastRun).toLocaleString() : '—'} · As of: {asOf ?? '—'}
                    </Typography>
                  );
                })()}
              >
                <SimpleStatTiles
                  size="md"
                  items={(() => {
                    const fm = getFormats(data.precompute);
                    const formats = ['TEST', 'ODI', 'T20I', 'T20'];
                    let ok = 0,
                      stale = 0,
                      missing = 0;
                    formats.forEach((f) => {
                      const st = readStatus(asObj((fm as Record<string, unknown>)[f]).status);
                      if (st === 'ok') ok++;
                      else if (st === 'stale') stale++;
                      else missing++;
                    });
                    return [
                      {
                        label: 'OK',
                        value: ok,
                        state: ok > 0 ? 'ok' : 'neutral',
                      },
                      {
                        label: 'Stale',
                        value: stale,
                        state: stale > 0 ? 'error' : 'neutral',
                      },
                      {
                        label: 'Missing',
                        value: missing,
                        state: missing > 0 ? 'error' : 'neutral',
                      },
                    ];
                  })()}
                />
                <OpsMatrix type="precompute" title="Precompute" data={data.precompute ?? {}} />
              </SectionCard>
            </Grid>
            <Grid item xs={12} md={6}>
              <SectionCard title="Exports">
                <SimpleStatTiles
                  size="md"
                  items={(() => {
                    const fm = getFormats(data.exports);
                    const formats = ['TEST', 'ODI', 'T20I', 'T20'];
                    let present = 0,
                      missing = 0;
                    const hasAnyExists = (files: unknown): boolean =>
                      Array.isArray(files) &&
                      files.some((e) => {
                        if (!e || typeof e !== 'object') return false;
                        const exists = (e as Record<string, unknown>).exists;
                        return exists === true;
                      });
                    formats.forEach((f) => {
                      const files = asObj((fm as Record<string, unknown>)[f]).files as unknown;
                      if (hasAnyExists(files)) present++;
                      else missing++;
                    });
                    return [
                      {
                        label: 'Present',
                        value: present,
                        state: present > 0 ? 'ok' : 'neutral',
                      },
                      {
                        label: 'Missing',
                        value: missing,
                        state: missing > 0 ? 'error' : 'neutral',
                      },
                    ];
                  })()}
                />
                <OpsMatrix type="exports" title="Exports" data={data.exports ?? {}} />
              </SectionCard>
            </Grid>
            <Grid item xs={12} md={6}>
              <SectionCard title="Artifacts">
                <SimpleStatTiles
                  size="md"
                  items={(() => {
                    const fm = getFormats(data.artifacts);
                    const formats = ['TEST', 'ODI', 'T20I', 'T20'];
                    let complete = 0,
                      missing = 0;
                    formats.forEach((f) => {
                      const row = asObj((fm as Record<string, unknown>)[f]);
                      const bat = asObj(row.batting);
                      const bowl = asObj(row.bowling);
                      const ok = bat?.exists === true && bowl?.exists === true;
                      if (ok) complete++;
                      else missing++;
                    });
                    return [
                      {
                        label: 'Complete',
                        value: complete,
                        state: complete > 0 ? 'ok' : 'neutral',
                      },
                      {
                        label: 'Missing',
                        value: missing,
                        state: missing > 0 ? 'error' : 'neutral',
                      },
                    ];
                  })()}
                />
                <OpsMatrix type="artifacts" title="Artifacts" data={data.artifacts ?? {}} />
              </SectionCard>
            </Grid>
            <Grid item xs={12} md={6}>
              <SectionCard
                title="Fielding Data"
                subtitle="Per-format and overall (unified) row counts."
              >
                {(() => {
                  const f = asObj(data.fielding);
                  const available = (f as Record<string, unknown>).available === true;
                  const rowsVal = (f as Record<string, unknown>).rows;
                  const totalRows = typeof rowsVal === 'number' ? rowsVal : undefined;
                  const overall = asObj((f as Record<string, unknown>).overall);
                  const overallRows = readNumber(overall.rows);
                  const fmtMap = getFormats(f);
                  const formatKeys = Object.keys(fmtMap).length > 0 ? FORMATS : [];
                  return (
                    <Stack spacing={1.5}>
                      <SimpleStatTiles
                        items={[
                          {
                            label: 'Available',
                            value: available,
                            state: available ? 'ok' : 'error',
                            title: available ? 'data available' : 'no data',
                          },
                          {
                            label: 'Total rows',
                            value: totalRows ?? overallRows ?? '—',
                            state: 'neutral',
                            title:
                              totalRows != null ? `${totalRows.toLocaleString()} rows` : 'unknown',
                          },
                        ]}
                      />
                      {formatKeys.length > 0 && (
                        <Stack spacing={0.75}>
                          <Typography variant="caption" fontWeight={600} color="text.secondary">
                            Per format
                          </Typography>
                          {FORMATS.map((fmt: FormatCode) => {
                            const row = asObj((fmtMap as Record<string, unknown>)[fmt]);
                            const r = readNumber(row.rows);
                            return (
                              <Stack
                                key={fmt}
                                direction="row"
                                justifyContent="space-between"
                                alignItems="center"
                              >
                                <Typography variant="body2">{fmt}</Typography>
                                <Typography variant="body2" sx={{ opacity: 0.9 }}>
                                  {r != null ? r.toLocaleString() : '—'} rows
                                </Typography>
                              </Stack>
                            );
                          })}
                        </Stack>
                      )}
                      <JsonCollapse data={data.fielding} summary="Show fielding details" />
                    </Stack>
                  );
                })()}
              </SectionCard>
            </Grid>
            <Grid item xs={12} md={6}>
              <SectionCard
                title="Weather Data"
                subtitle={<span>Summary of weather data availability and stats.</span>}
              >
                {(() => {
                  const w = asObj(data.weather);
                  const available = (w as Record<string, unknown>).available === true;
                  const rowsVal = (w as Record<string, unknown>).rows;
                  const rows = typeof rowsVal === 'number' ? rowsVal : undefined;
                  return (
                    <SimpleStatTiles
                      items={[
                        {
                          label: 'Available',
                          value: available,
                          state: available ? 'ok' : 'error',
                          title: available ? 'data available' : 'no data',
                        },
                        {
                          label: 'Rows',
                          value: rows ?? '—',
                          state: 'neutral',
                          title: rows != null ? `${rows} rows` : 'unknown',
                        },
                      ]}
                    />
                  );
                })()}
                <JsonCollapse data={data.weather} summary="Show weather details" />
              </SectionCard>
            </Grid>
          </Grid>

          {/* Removed global suggestions block to keep suggestions within each section */}

          <SectionCard title="Match Type Hierarchy">
            <OpsFormatHierarchy hierarchy={data.hierarchy} />
          </SectionCard>

          <JsonCollapse data={data} summary="Show raw JSON payload" />
        </Stack>
      )}
    </Stack>
  );
};

export default OpsStatusTab;
