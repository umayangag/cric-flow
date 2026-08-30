import React from 'react';
import { Typography } from '@mui/material';
import SectionCard from './common/SectionCard';
import SimpleStatTiles from './common/SimpleStatTiles';
import type { DatasetStatus } from '../types';
import { formatBytes, formatWhen } from '../utils/format';

/**
 * What is in the dataset directory on the server.
 *
 * The point of this section is that "is there any data on this box?" should be
 * answerable from the browser. It reports the directory the importer will actually
 * read — same path, same non-recursive rule — so the count here is the count import
 * will find.
 */
const OpsDatasetSection: React.FC<{ dataset?: DatasetStatus }> = ({ dataset }) => {
  if (!dataset) return null;

  const missing = !dataset.exists;
  const empty = dataset.exists && dataset.match_files === 0;

  return (
    <SectionCard
      title="Dataset"
      subtitle="Cricsheet match files on the server, in the directory Import reads from."
    >
      <Typography variant="body2" component="div" sx={{ wordBreak: 'break-all' }}>
        Path: <strong>{dataset.path || '—'}</strong>
      </Typography>
      <SimpleStatTiles
        size="md"
        items={[
          {
            label: 'Present',
            value: !missing,
            state: missing ? 'error' : 'ok',
            title: missing ? 'Directory does not exist' : 'Directory exists',
          },
          {
            label: 'Match files',
            value: dataset.match_files ?? 0,
            state: empty ? 'error' : 'neutral',
            title: 'Files Import will read (.json, not recursive)',
          },
          {
            label: 'Size',
            value: formatBytes(dataset.bytes ?? 0),
            state: 'neutral',
          },
          {
            label: 'Newest',
            value: formatWhen(dataset.newest_modified),
            state: 'neutral',
            title: dataset.newest_file || 'No match files',
          },
        ]}
      />
      {(missing || empty) && (
        <Typography variant="body2" color="error" component="div">
          {missing
            ? 'That directory does not exist on the server.'
            : 'No .json match files there — Import would find nothing.'}{' '}
          Point <code>{dataset.env_var || 'GO_APP_CRICSHEET_DIR'}</code> (or{' '}
          <code>inputs.cricsheet_dir</code> in config) at the directory holding the match files.
          {dataset.error ? ` (${dataset.error})` : ''}
        </Typography>
      )}
    </SectionCard>
  );
};

export default OpsDatasetSection;
