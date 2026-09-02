import React from 'react';
import { Grid, Stack, Typography } from '@mui/material';
import JsonCollapse from './common/JsonCollapse';
import SimpleStatTiles from './common/SimpleStatTiles';
import SectionCard from './common/SectionCard';
import OpsRunsPanel from './OpsRunsPanel';
import { asObj, getFormats, readNumber } from '../utils/opsStatusHelpers';
import type { OpsStatus } from '../utils/opsStatusHelpers';

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
  const formatList = formats;
  return (
    <Grid container spacing={2} alignItems="stretch">
      <Grid item xs={12} md={6}>
        <OpsRunsPanel data={data} />
      </Grid>
      <Grid item xs={12} md={6}>
        <SectionCard title="Fielding Data" subtitle="Per-format and overall (unified) row counts.">
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
    </Grid>
  );
};

export default OpsStatusDataGrid;
