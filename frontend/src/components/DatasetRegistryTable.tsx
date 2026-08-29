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
 * so it stays true when files are put there by other means. That leaves two ways for
 * a table to come back with nothing marked, and both are said out loud, because a
 * silently unmarked table reads as "the row you can see is the one in use" — a
 * different and wrong answer:
 *
 *   - `live_sha256` set, no row marked: the directory holds a dataset the registry
 *     has never seen.
 *   - `live_sha256` empty: the directory has no manifest at all, so nothing can be
 *     matched against it. A dataset extracted before the registry existed, or files
 *     copied in directly, looks like this.
 */
const DatasetRegistryTable: React.FC<Props> = ({ registry }) => {
  const datasets = registry?.datasets ?? [];
  const liveSha = registry?.live_sha256 ?? '';
  const nothingIsMarkedLive = !datasets.some((d) => d.live);
  // A digest with no matching row: the directory holds something unrecorded.
  const liveIsUnknown = liveSha !== '' && nothingIsMarkedLive;
  // No digest at all: there is no manifest to match rows against. Only worth saying
  // when there are rows, since an empty registry has its own message below.
  const liveIsUnmanifested = liveSha === '' && datasets.length > 0;

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

      {liveIsUnmanifested && (
        <Typography variant="body2" color="warning.main">
          The dataset directory has no manifest, so none of these rows can be identified as the data
          in use — a row below is somewhere this box has fetched from, not necessarily what it
          imported. A manifest is written on extraction, so the next Extract will settle it.
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
