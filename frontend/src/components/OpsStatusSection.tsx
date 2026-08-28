import React from 'react';
import OpsPipelineGraph from './OpsPipelineGraph';
import PipelineProgressPanel from './PipelineProgressPanel';
import RunPlanPanel from './RunPlanPanel';
import OpsMigrationsTable from './OpsMigrationsTable';
import OpsTableStats from './OpsTableStats';
import OpsStatusDetailsGrid from './OpsStatusDetailsGrid';
import OpsDatasetSection from './OpsDatasetSection';
import Stack from '@mui/material/Stack';
import Button from '@mui/material/Button';
import Typography from '@mui/material/Typography';
import Grid from '@mui/material/Grid';
import StatusPill from './common/StatusPill';
import JsonCollapse from './common/JsonCollapse';
import SimpleStatTiles from './common/SimpleStatTiles';
import SectionCard from './common/SectionCard';
import { formatWhen } from '../utils/format';
import ErrorNotice from './common/ErrorNotice';
import { asObj } from '../utils/opsStatusHelpers';
import type { OpsStatus } from '../utils/opsStatusHelpers';
import { TableStat } from '../types';

// Re-export OpsStatus so existing consumers (OpsStatusTab, OpsPipelineGraph) don't break
export type { OpsStatus } from '../utils/opsStatusHelpers';

const REFRESH_MS = 15000;

export interface OpsStatusSectionProps {
  data: OpsStatus | null;
  error: string | null;
  loading: boolean;
  lastUpdated: string;
  onRefresh: () => void;
}

export const OpsStatusSection: React.FC<OpsStatusSectionProps> = ({
  data,
  error,
  loading,
  lastUpdated,
  onRefresh,
}) => (
  <Stack spacing={2}>
    <Stack direction="row" spacing={1} alignItems="center">
      <Button
        variant="contained"
        onClick={onRefresh}
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

    <ErrorNotice error={error} title="Could not fetch /ops/status" />

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
          <OpsPipelineGraph data={data} onRefresh={onRefresh} />
          <RunPlanPanel onRefresh={onRefresh} />
          <PipelineProgressPanel
            pipelineRunning={Object.values(asObj(data.pipeline?.steps ?? {})).some(
              (s) => asObj(s).running === true,
            )}
            onRefresh={onRefresh}
          />
        </SectionCard>
        <OpsDatasetSection dataset={data.dataset} />

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
                  {formatWhen(asObj(data?.db).last_match_import_at as string | undefined)}
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

        <OpsStatusDetailsGrid data={data} />
      </Stack>
    )}
  </Stack>
);

export default OpsStatusSection;
