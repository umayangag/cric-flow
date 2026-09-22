import React from 'react';
import { Stack, Typography } from '@mui/material';
import StatusPill from './common/StatusPill';
import type { TeamLineage } from '../utils/opsStatusHelpers';

interface ClubLineageLineProps {
  lineage: TeamLineage;
}

/**
 * Whether the clubs that renamed have been joined back up, on the Database card.
 *
 * A club whose two rows are never linked is two clubs everywhere downstream — two Elo
 * histories, two form series, two head-to-head records — and nothing else in the system
 * says so. An import that stopped before settlement left exactly that (IMPORT-07), so the
 * count belongs where an operator already looks rather than behind a SQL prompt.
 */
const ClubLineageLine: React.FC<ClubLineageLineProps> = ({ lineage }) => {
  if (lineage.status === 'unknown') {
    return (
      <Typography variant="body2" component="div">
        Club lineage: <strong>not available</strong>
      </Typography>
    );
  }
  return (
    <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
      <Typography variant="body2" component="div">
        Club lineage:{' '}
        <strong>
          {lineage.linked ?? 0} of {lineage.renamesConfigured ?? 0} renamed clubs linked
        </strong>
      </Typography>
      {lineage.status === 'incomplete' && (
        <StatusPill
          state="error"
          label={`${lineage.unlinkedRenames.length} unlinked — re-run the import`}
        />
      )}
    </Stack>
  );
};

export default ClubLineageLine;
