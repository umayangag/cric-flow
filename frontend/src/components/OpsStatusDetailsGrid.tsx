import React from 'react';
import { Grid, Stack, Typography } from '@mui/material';
import StatusPill from './common/StatusPill';
import SectionCard from './common/SectionCard';
import OpsStatusDataGrid from './OpsStatusDataGrid';
import JsonCollapse from './common/JsonCollapse';
import { useCanonicalFormats } from '../hooks/useCanonicalFormats';
import {
  asObj,
  getFormats,
  readCompletenessStatus,
  readNumber,
  readFreshness,
} from '../utils/opsStatusHelpers';
import type { OpsStatus } from '../utils/opsStatusHelpers';
import FreshnessCard from './FreshnessCard';

interface OpsStatusDetailsGridProps {
  data: OpsStatus;
}

const OpsStatusDetailsGrid: React.FC<OpsStatusDetailsGridProps> = ({ data }) => {
  const { formats, loading: formatsLoading, error: formatsError } = useCanonicalFormats();
  const freshness = readFreshness(data);
  const formatList = formats.length > 0 ? formats : Object.keys(freshness.database);

  return (
    <>
      {/* Two blocks per row below Database */}
      <Grid container spacing={2} alignItems="stretch">
        <Grid item xs={12} md={6}>
          <FreshnessCard freshness={freshness} formats={formatList} formatsError={formatsError} />
        </Grid>
        <Grid item xs={12} md={6}>
          {(() => {
            const comp = asObj((data as { db_completeness?: unknown })?.db_completeness);
            const overallObj = asObj(comp.overall);
            const overallSt = readCompletenessStatus(overallObj.status);
            const overallLast30 = readNumber(overallObj.matches_last_30d);
            const fm = getFormats(comp);
            const completenessFormatList = formats.length > 0 ? formats : Object.keys(fm);
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
                ) : formatsError ? (
                  <Typography variant="body2" color="error">
                    {formatsError}
                  </Typography>
                ) : (
                  <Stack spacing={1}>
                    {completenessFormatList.map((f) => {
                      const row = asObj(fm[f]);
                      const st = readCompletenessStatus(row.status);
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

      <OpsStatusDataGrid
        data={data}
        formats={formats}
        formatsLoading={formatsLoading}
        formatsError={formatsError}
      />

      <JsonCollapse data={data} summary="Show raw JSON payload" />
    </>
  );
};

export default OpsStatusDetailsGrid;
