import React from 'react';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import Grid from '@mui/material/Grid';
import StatusPill from './common/StatusPill';
import SectionCard from './common/SectionCard';
import OpsFormatHierarchy from './OpsFormatHierarchy';
import OpsStatusDataGrid from './OpsStatusDataGrid';
import JsonCollapse from './common/JsonCollapse';
import {
  FORMATS,
  asObj,
  getFormats,
  readStatus,
  readNumber,
} from '../utils/opsStatusHelpers';
import type { FormatCode, OpsStatus } from '../utils/opsStatusHelpers';

interface OpsStatusDetailsGridProps {
  data: OpsStatus;
}

const OpsStatusDetailsGrid: React.FC<OpsStatusDetailsGridProps> = ({ data }) => (
  <>
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
              title="DB Data Completeness"
              subtitle={
                <Stack direction="row" spacing={1} alignItems="center">
                  <Typography variant="body2" component="div">
                    Overall
                  </Typography>
                  <StatusPill state={overallSt} label={overallSt} />
                  {overallLast30 != null && (
                    <Typography variant="body2" sx={{ opacity: 0.85 }}>
                      {overallLast30.toLocaleString()} matches (last 30d)
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
                    const last30 = readNumber(row.matches_last_30d);
                    const total = readNumber(row.total_matches);
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
                          {last30 != null && <strong>{last30} last 30d</strong>}
                          {last30 != null && total != null && ' · '}
                          {total != null && `${total.toLocaleString()} total`}
                          {last30 == null && total == null && 'not available'}
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
    </Grid>

    <OpsStatusDataGrid data={data} />

    <SectionCard title="Match Type Hierarchy">
      <OpsFormatHierarchy hierarchy={data.hierarchy} />
    </SectionCard>

    <JsonCollapse data={data} summary="Show raw JSON payload" />
  </>
);

export default OpsStatusDetailsGrid;
