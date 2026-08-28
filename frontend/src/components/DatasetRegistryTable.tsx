import React from 'react';
import { Box } from '@mui/material';
import Chip from '@mui/material/Chip';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import SectionCard from './common/SectionCard';
import { formatBytes, formatWhen, shortDigest } from '../utils/format';
import type { DatasetRegistryResponse } from '../types';

type Props = { registry?: DatasetRegistryResponse };

/** Where a dataset came from, or an honest statement that nobody knows. */
const Source: React.FC<{ feed?: string; url?: string }> = ({ feed, url }) => {
  if (!feed && !url) {
    return (
      <Typography variant="caption" color="text.secondary" sx={{ fontStyle: 'italic' }}>
        placed by hand
      </Typography>
    );
  }
  return (
    <Tooltip title={url || ''}>
      <Typography variant="body2" component="span">
        {feed || 'URL'}
      </Typography>
    </Tooltip>
  );
};

/**
 * Every dataset this box has acquired, newest first, with the live one marked.
 *
 * "Live" is derived from the dataset directory's manifest rather than a stored flag,
 * so it stays true when files are put there by other means. The consequence shows up
 * here: `live_sha256` can be set while no row is marked, which means the directory
 * holds a dataset the registry has never seen. That is called out rather than
 * rendered as a table with nothing highlighted, because a silently unmarked table
 * looks like "nothing is live", which is a different and wrong answer.
 */
const DatasetRegistryTable: React.FC<Props> = ({ registry }) => {
  const datasets = registry?.datasets ?? [];
  const liveSha = registry?.live_sha256 ?? '';
  const liveIsUnknown = liveSha !== '' && !datasets.some((d) => d.live);

  return (
    <SectionCard
      title="Dataset registry"
      subtitle="Every dataset acquired on this box. The archive digest is the identity — Cricsheet reuses filenames across releases."
    >
      {liveIsUnknown && (
        <Typography variant="body2" color="warning.main">
          The dataset directory holds an archive this registry has never seen (sha256{' '}
          {shortDigest(liveSha)}). It was probably extracted before the registry existed, or the
          files were placed there directly.
        </Typography>
      )}

      {datasets.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          {registry
            ? 'No datasets recorded yet. Fetch one to start the registry.'
            : 'Registry unavailable — the database may be down.'}
        </Typography>
      ) : (
        <Box sx={{ overflowX: 'auto' }}>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Archive</TableCell>
                <TableCell>Source</TableCell>
                <TableCell>Digest</TableCell>
                <TableCell align="right">Size</TableCell>
                <TableCell align="right">Match files</TableCell>
                <TableCell>Fetched</TableCell>
                <TableCell>Extracted</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {datasets.map((d) => (
                <TableRow key={d.id} selected={d.live}>
                  <TableCell sx={{ whiteSpace: 'nowrap' }}>
                    {d.filename}
                    {d.live && <Chip size="small" color="success" label="live" sx={{ ml: 1 }} />}
                  </TableCell>
                  <TableCell>
                    <Source feed={d.feed} url={d.source_url} />
                  </TableCell>
                  <TableCell>
                    <Tooltip title={d.sha256}>
                      <Typography variant="caption" fontFamily="monospace">
                        {shortDigest(d.sha256)}
                      </Typography>
                    </Tooltip>
                  </TableCell>
                  <TableCell align="right">{formatBytes(d.bytes)}</TableCell>
                  <TableCell align="right">
                    {/* Never extracted is not zero match files. */}
                    {d.extracted_at ? (d.match_files ?? 0) : '—'}
                  </TableCell>
                  <TableCell sx={{ whiteSpace: 'nowrap' }}>{formatWhen(d.fetched_at)}</TableCell>
                  <TableCell sx={{ whiteSpace: 'nowrap' }}>{formatWhen(d.extracted_at)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Box>
      )}
    </SectionCard>
  );
};

export default DatasetRegistryTable;
