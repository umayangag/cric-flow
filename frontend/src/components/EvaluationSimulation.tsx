import React from 'react';
import {
  Chip,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import type { EvaluationFormatReport } from '../types';
import { MetricLabel } from './common/MetricInfo';
import MetricValue from './common/MetricValue';
import { formatStat } from '../utils/evaluationReport';

const totalLabels: Record<string, string> = {
  first_innings: 'First innings',
  chase: 'Chase',
};

/**
 * E2: is the simulated P(win) a probability, or a description of the draws?
 *
 * The decision is made on the folds and served as a constant; what is shown here is the
 * measurement behind it, plus the coverage and width of the simulated totals against what
 * the innings actually were. The dispersion ratio above 1 means the simulator is
 * under-dispersed — actual totals scatter more than its draws do.
 */
const EvaluationSimulation: React.FC<{ report: EvaluationFormatReport }> = ({ report }) => {
  const decision = report.simulation_decision;
  const folds = report.walk_forward.summary.simulation;
  const locked = report.locked.simulation;

  if (folds?.skipped_reason || locked?.skipped_reason) {
    return (
      <Paper variant="outlined" sx={{ p: 2, mb: 3 }}>
        <Typography variant="subtitle1" fontWeight={600} gutterBottom>
          Simulation (E2)
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {folds?.skipped_reason ?? locked?.skipped_reason}
        </Typography>
      </Paper>
    );
  }

  const totals = locked?.totals ?? folds?.totals ?? {};

  return (
    <Paper variant="outlined" sx={{ mb: 3 }}>
      <Stack
        direction="row"
        spacing={1}
        alignItems="center"
        flexWrap="wrap"
        sx={{ px: 2, py: 1.5 }}
      >
        <Typography variant="subtitle1" fontWeight={600}>
          Simulation (E2)
        </Typography>
        <Chip
          size="small"
          color={decision.simulated_win_probability_within_tolerance ? 'success' : 'default'}
          label={
            decision.simulated_win_probability_within_tolerance
              ? 'simulated P(win) is a probability'
              : 'simulated P(win) is a description'
          }
        />
        <Chip
          size="small"
          variant="outlined"
          label={decision.served ? 'served as the headline' : 'display model is the headline'}
        />
        {decision.shared_factor && (
          <Chip size="small" variant="outlined" label="shared match factor on" />
        )}
      </Stack>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', px: 2, pb: 1 }}>
        {decision.reason}
      </Typography>
      <TableContainer>
        <Table size="small" aria-label="simulated totals">
          <TableHead>
            <TableRow>
              <TableCell>Total</TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="coverage_80" label="10–90 coverage" />
              </TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="width_80" label="10–90 width" />
              </TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="dispersion_ratio" label="Dispersion ratio" />
              </TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="median_mae" label="Median MAE" />
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {Object.entries(totals).map(([name, entry]) => (
              <TableRow key={name} hover>
                <TableCell>{totalLabels[name] ?? name}</TableCell>
                <TableCell align="right">
                  <MetricValue metricKey="coverage_80" value={entry.coverage_80} as="share" />
                </TableCell>
                <TableCell align="right">{formatStat(entry.width_80, 1)}</TableCell>
                <TableCell align="right">
                  <MetricValue
                    metricKey="dispersion_ratio"
                    value={entry.dispersion_ratio}
                    digits={2}
                  />
                </TableCell>
                <TableCell align="right">{formatStat(entry.median_mae, 1)}</TableCell>
              </TableRow>
            ))}
            <TableRow>
              <TableCell>
                <MetricLabel metricKey="brier" label="Brier, simulated vs display" />
              </TableCell>
              <TableCell align="right" colSpan={4}>
                <MetricValue
                  metricKey="brier"
                  value={locked?.win?.brier?.simulated ?? folds?.win?.brier?.simulated}
                />{' '}
                vs{' '}
                <MetricValue
                  metricKey="brier"
                  value={locked?.win?.brier?.display ?? folds?.win?.brier?.display}
                />
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </TableContainer>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', px: 2, py: 1 }}>
        Totals are the locked window where it was scored, otherwise the fold means. The chase
        orientation is a recorded constant: {decision.chase_orientation ?? 'unset'}.
      </Typography>
    </Paper>
  );
};

export default EvaluationSimulation;
