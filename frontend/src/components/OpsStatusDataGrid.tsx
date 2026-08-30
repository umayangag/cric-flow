import React from 'react';
import { Grid, Stack, Typography } from '@mui/material';
import StatusPill from './common/StatusPill';
import JsonCollapse from './common/JsonCollapse';
import SimpleStatTiles from './common/SimpleStatTiles';
import SectionCard from './common/SectionCard';
import OpsMatrix from './OpsMatrix';
import { asObj, getFormats, readStatus, readNumber } from '../utils/opsStatusHelpers';
import type { ExportFile, OpsStatus } from '../utils/opsStatusHelpers';

interface OpsStatusDataGridProps {
  data: OpsStatus;
  formats: string[];
  formatsLoading?: boolean;
  formatsError?: string | null;
}

const OpsStatusDataGrid: React.FC<OpsStatusDataGridProps> = ({
  data,
  formats,
  formatsLoading: _formatsLoading,
  formatsError,
}) => {
  const formatList = formats.length > 0 ? formats : Object.keys(getFormats(data.precompute));
  return (
    <>
      <Grid container spacing={2} alignItems="stretch">
        <Grid item xs={12} md={6}>
          {(() => {
            const pre = asObj(data.precompute);
            const fm = getFormats(pre);
            return (
              <SectionCard title="Precompute">
                <OpsMatrix type="precompute" title="Precompute" data={pre} formats={formatList} />
                {!formatsError && Object.keys(fm).length > 0 && (
                  <Stack spacing={0.75} sx={{ mt: 1 }}>
                    {formatList.map((f) => {
                      const row = asObj(fm[f]);
                      const st = readStatus(row.status);
                      return (
                        <Stack
                          key={f}
                          direction="row"
                          alignItems="center"
                          justifyContent="space-between"
                        >
                          <Typography variant="body2">{f}</Typography>
                          <StatusPill state={st} label={st} />
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
            const exp = asObj(data.exports);
            const fm = getFormats(exp);
            return (
              <SectionCard title="Exports">
                <OpsMatrix type="exports" title="Exports" data={exp} formats={formatList} />
                {!formatsError && Object.keys(fm).length > 0 && (
                  <Stack spacing={0.75} sx={{ mt: 1 }}>
                    {formatList.map((f) => {
                      const row = asObj(fm[f]);
                      const files = Array.isArray(row.files) ? (row.files as ExportFile[]) : [];
                      const allExist = files.length > 0 && files.every((x) => x.exists);
                      const st = allExist ? 'ok' : files.length > 0 ? 'stale' : 'missing';
                      return (
                        <Stack
                          key={f}
                          direction="row"
                          alignItems="center"
                          justifyContent="space-between"
                        >
                          <Typography variant="body2">{f}</Typography>
                          <StatusPill
                            state={st}
                            label={`${files.filter((x) => x.exists).length}/${files.length} files`}
                          />
                        </Stack>
                      );
                    })}
                  </Stack>
                )}
              </SectionCard>
            );
          })()}
        </Grid>
      </Grid>

      <Grid container spacing={2} alignItems="stretch">
        <Grid item xs={12} md={6}>
          <SectionCard title="ML Artifacts">
            <SimpleStatTiles
              items={(() => {
                const art = asObj(data.artifacts);
                const fm = getFormats(art);
                let loaded = 0;
                let missing = 0;
                formatList.forEach((f) => {
                  const row = asObj(fm[f]);
                  const bat = asObj(row.batting);
                  const bowl = asObj(row.bowling);
                  if (bat.loaded) loaded++;
                  else missing++;
                  if (bowl.loaded) loaded++;
                  else missing++;
                });
                return [
                  { label: 'Loaded', value: loaded, state: loaded > 0 ? 'ok' : 'neutral' },
                  { label: 'Missing', value: missing, state: missing > 0 ? 'error' : 'neutral' },
                ];
              })()}
            />
            <OpsMatrix
              type="artifacts"
              title="Artifacts"
              data={data.artifacts ?? {}}
              formats={formatList}
            />
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
              const formatKeys = Object.keys(fmtMap).length > 0 ? formatList : [];
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
                        title: totalRows != null ? `${totalRows.toLocaleString()} rows` : 'unknown',
                      },
                    ]}
                  />
                  {formatKeys.length > 0 && !formatsError && (
                    <Stack spacing={0.75}>
                      <Typography variant="caption" fontWeight={600} color="text.secondary">
                        Per format
                      </Typography>
                      {formatList.map((fmt) => {
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
    </>
  );
};

export default OpsStatusDataGrid;
