import React from 'react';
import {
  Box,
  Chip,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import SectionCard from './common/SectionCard';
import { formatCount, formatPercent, formatWhen } from '../utils/format';
import type { BiographyCoverageResponse, BiographyCoverageRow } from '../types';

type Props = { coverage?: BiographyCoverageResponse };

/** Below this share of appearances a fact is too sparse to build on; the row says so. */
const THIN_COVERAGE = 0.5;

/** One percentage cell, marked when the coverage is too thin to be worth building on. */
const Share: React.FC<{ numerator: number; denominator: number }> = ({
  numerator,
  denominator,
}) => {
  const fraction = denominator > 0 ? numerator / denominator : 0;
  return (
    <Typography
      variant="body2"
      component="span"
      color={fraction < THIN_COVERAGE ? 'text.secondary' : 'text.primary'}
    >
      {formatPercent(fraction)}
    </Typography>
  );
};

const CoverageCells: React.FC<{ row: BiographyCoverageRow }> = ({ row }) => (
  <>
    <TableCell align="right">{formatCount(row.appearances)}</TableCell>
    <TableCell align="right">
      <Share numerator={row.matched} denominator={row.appearances} />
    </TableCell>
    <TableCell align="right">
      <Share numerator={row.birth_date} denominator={row.appearances} />
    </TableCell>
    <TableCell align="right">
      <Share numerator={row.bowling_style} denominator={row.appearances} />
    </TableCell>
    <TableCell align="right">
      <Share numerator={row.batting_hand} denominator={row.appearances} />
    </TableCell>
    <TableCell align="right">
      <Share numerator={row.career_end} denominator={row.appearances} />
    </TableCell>
  </>
);

/**
 * How much of the archive the acquired player biographies cover (X-1a).
 *
 * It sits beside the dataset registry because it answers the same kind of question: what
 * data is on this box, and how good is it? A coverage figure that lived only in a
 * backfill's log would be invisible exactly when it matters — after an import has added
 * players nobody has looked up yet, which shows here as appearances that were never
 * attempted rather than as a match that failed.
 *
 * Every figure is weighted by appearances rather than by players, because that is the
 * share a feature reading these facts would actually see.
 */
const BiographyCoverageTable: React.FC<Props> = ({ coverage }) => {
  const rows = coverage?.rows ?? [];
  const total = coverage?.total;
  const neverRun = coverage !== undefined && !coverage.last_fetched_at;

  return (
    <SectionCard
      title="Player biography coverage"
      subtitle={
        <>
          Dates of birth, handedness, bowling style and career end acquired from Wikidata (
          {coverage?.source_license ?? 'CC0-1.0'}), joined through Cricsheet&apos;s people register
          and the ESPNcricinfo player id. Weighted by appearances — the share of fielded
          player-sides a feature could read. Refreshed by <code>make player-biographies</code>, a
          step beside the cadence.
        </>
      }
    >
      {!coverage && (
        <Typography variant="body2" color="text.secondary">
          Coverage unavailable — the database may be down.
        </Typography>
      )}

      {neverRun && (
        <Typography variant="body2" color="warning.main">
          No biographies have been acquired on this box. Run <code>make player-biographies</code>;
          until then every figure below is zero because nothing was looked up, which is a different
          state from a source that was asked and had nothing.
        </Typography>
      )}

      {coverage && !neverRun && (
        <Typography variant="body2" color="text.secondary">
          Acquired {formatWhen(coverage.last_fetched_at)}.
        </Typography>
      )}

      {rows.length > 0 && (
        <Box sx={{ overflowX: 'auto' }}>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Format</TableCell>
                <TableCell>Gender</TableCell>
                <TableCell align="right">Appearances</TableCell>
                <TableCell align="right">Matched</TableCell>
                <TableCell align="right">Date of birth</TableCell>
                <TableCell align="right">Bowling style</TableCell>
                <TableCell align="right">Batting hand</TableCell>
                <TableCell align="right">Career end</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {rows.map((row) => (
                <TableRow key={`${row.format}-${row.gender}`}>
                  <TableCell sx={{ whiteSpace: 'nowrap' }}>{row.format}</TableCell>
                  <TableCell>{row.gender}</TableCell>
                  <CoverageCells row={row} />
                </TableRow>
              ))}
              {total && (
                <TableRow selected>
                  <TableCell sx={{ whiteSpace: 'nowrap' }}>All</TableCell>
                  <TableCell>all</TableCell>
                  <CoverageCells row={total} />
                </TableRow>
              )}
            </TableBody>
          </Table>
        </Box>
      )}

      {coverage && coverage.unmatched.length > 0 && (
        <Typography variant="body2" color="text.secondary" component="div">
          Largest gap:{' '}
          <Chip
            size="small"
            label={`${coverage.unmatched[0].name} — ${formatCount(coverage.unmatched[0].appearances)} appearances`}
          />{' '}
          and {formatCount(coverage.unmatched.length - 1)} more listed in{' '}
          <code>docs/player-biography-coverage.md</code>, in the order a curated override would pay
          off.
        </Typography>
      )}
    </SectionCard>
  );
};

export default BiographyCoverageTable;
