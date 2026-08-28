import React, { useMemo } from 'react';
import Chip from '@mui/material/Chip';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { formatWhen } from '../utils/format';
import { derivePipelineSteps } from '../utils/pipelineSteps';
import type { AccuracyTrendItem, Migration } from '../types';

/**
 * The `data_migrations` commands whose runs change what a prediction would say.
 *
 * Taken from the step list rather than typed here, so it cannot name a command the
 * backend does not write. Precompute is deliberately absent: it rebuilds feature
 * snapshots, but nothing is scored differently until an export and a retrain follow.
 */
const CHANGES_THE_ANSWER = new Set(
  derivePipelineSteps(null)
    .filter(
      (step) =>
        step.id.startsWith('train_') ||
        step.id === 'auto_tune' ||
        step.id === 'import' ||
        step.id === 'export',
    )
    .map((step) => step.migrationCommand),
);

type Props = {
  results: AccuracyTrendItem[];
  migrations: Migration[];
  loading: boolean;
};

/** What kind of change a run was, in the words an operator would use. */
function changeLabel(command: string): string {
  const step = derivePipelineSteps(null).find((s) => s.migrationCommand === command);
  switch (step?.id) {
    case 'auto_tune':
      return 'auto-tune';
    case 'import':
      return 'new data';
    case 'export':
      return 'new export';
    default:
      return step?.id.startsWith('train_') ? 'retrain' : command;
  }
}

/**
 * The runs that happened inside the plotted window (ops O-4, consumer W5-2).
 *
 * The accuracy trend is per-match error over time, and on its own it cannot say why a
 * number moved. It is not a chart of the model changing — it is a chart of *matches*,
 * scored by whatever model is loaded now. What actually changed between two points is
 * in run history, and it is a different table entirely.
 *
 * So this does not pretend to overlay them on one axis, which would imply each point
 * was produced by the model of its day. It lists the runs that fall inside the plotted
 * range, newest first, so a step in the trend can be lined up with the retrain,
 * auto-tune or dataset change that plausibly caused it — and so a flat trend across a
 * window with no runs in it reads as "nothing changed" rather than as a mystery.
 */
const AccuracyTrendRuns: React.FC<Props> = ({ results, migrations, loading }) => {
  const { from, to } = useMemo(() => {
    const dates = results
      .map((r) => r.match_date)
      .filter(Boolean)
      .sort();
    return { from: dates[0], to: dates[dates.length - 1] };
  }, [results]);

  const runs = useMemo(() => {
    if (!from || !to) return [];
    return (
      migrations
        .filter((m) => CHANGES_THE_ANSWER.has(m.command))
        .filter((m) => m.status === 'COMPLETED')
        // The window is the plotted match dates. A run outside it cannot explain a
        // movement inside it, and listing every run there has ever been would bury the
        // few that can.
        .filter((m) => m.started_at >= from && m.started_at <= `${to}T23:59:59Z`)
        .sort((a, b) => b.started_at.localeCompare(a.started_at))
    );
  }, [migrations, from, to]);

  if (results.length === 0) return null;

  if (loading) {
    return (
      <Typography variant="body2" color="text.secondary" sx={{ mt: 2, fontStyle: 'italic' }}>
        Loading run history…
      </Typography>
    );
  }

  if (runs.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
        No retrain, auto-tune or dataset change ran between {formatWhen(from)} and {formatWhen(to)}.
        Movement in the trend above is the matches differing, not the model.
      </Typography>
    );
  }

  return (
    <Stack spacing={1} sx={{ mt: 2 }}>
      <Typography variant="subtitle2">What changed during this window</Typography>
      <Typography variant="body2" color="text.secondary">
        Runs that could have moved these numbers, from run history. The trend is per-match error
        scored by the models loaded now — this is what happened to those models in between.
      </Typography>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell>When</TableCell>
            <TableCell>Change</TableCell>
            <TableCell>Run</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {runs.map((run) => (
            <TableRow key={run.id}>
              <TableCell sx={{ whiteSpace: 'nowrap' }}>{formatWhen(run.started_at)}</TableCell>
              <TableCell>
                <Chip size="small" variant="outlined" label={changeLabel(run.command)} />
              </TableCell>
              <TableCell>{run.command}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Stack>
  );
};

export default AccuracyTrendRuns;
