import React from 'react';
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
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
import ErrorNotice from './common/ErrorNotice';
import { MetricLabel } from './common/MetricInfo';
import TrackRecordReliabilityPlot from './trackRecord/TrackRecordReliabilityPlot';
import TrackRecordList, { POPULATION_LABELS, STATE_LABELS } from './trackRecord/TrackRecordList';
import { useTrackRecord, type HarnessFigures } from '../hooks/useTrackRecord';
import {
  PREDICTION_STATES,
  SIMULATOR_POPULATIONS,
  type TrackRecordCoverageScore,
  type TrackRecordWinScore,
} from '../types';

function stat(value: number | null | undefined, digits = 3): string {
  return value == null ? '—' : value.toFixed(digits);
}

/** "3 of 4 (75.0%)": the numerator and the denominator before the share. */
function coverage(score: TrackRecordCoverageScore): string {
  if (score.n === 0) return 'n=0';
  return `${score.covered} of ${score.n} (${((score.coverage ?? 0) * 100).toFixed(1)}%)`;
}

function share(value: number | null | undefined): string {
  return value == null ? '—' : `${(value * 100).toFixed(1)}%`;
}

const WinRow: React.FC<{ label: string; score: TrackRecordWinScore; harness?: HarnessFigures }> = ({
  label,
  score,
  harness,
}) => (
  <TableRow hover>
    <TableCell>{label}</TableCell>
    <TableCell align="right">{score.n}</TableCell>
    <TableCell align="right">{stat(score.brier)}</TableCell>
    <TableCell align="right">
      {stat(score.base_rate_brier)}
      {score.base_rate != null && (
        <Typography variant="caption" color="text.secondary" display="block">
          rate {stat(score.base_rate, 2)}
        </Typography>
      )}
    </TableCell>
    <TableCell align="right">{harness ? stat(harness.displayBrier) : '—'}</TableCell>
    <TableCell align="right">{harness ? stat(harness.baseRateBrier) : '—'}</TableCell>
    <TableCell>
      {harness ? (
        <Typography variant="caption" color="text.secondary">
          {harness.source}
          {harness.n != null ? `, n=${Math.round(harness.n)}` : ''}
        </Typography>
      ) : (
        <Typography variant="caption" color="text.secondary">
          no harness figure
        </Typography>
      )}
    </TableCell>
  </TableRow>
);

/**
 * The internal track record (P2-4): every stored prediction in its state, the scored ones
 * against what happened, computed on read from the store and the match tables.
 *
 * What it is not, and the surface says so: evidence. One operator's record is tens of
 * predictions, so every figure carries its n, nothing is coloured by a threshold, and the
 * harness's locked-window or fold figures sit beside the record's, labelled, so a reader
 * sees the record against the number the model was accepted on. Nothing is filtered by
 * outcome. A miss on the record is the record working.
 */
const TrackRecordTab: React.FC = () => {
  const { record, loading, error, reload, harnessByFormat, harnessGeneratedAt, harnessError } =
    useTrackRecord();

  if (loading && !record) {
    return (
      <Stack direction="row" spacing={1} alignItems="center" sx={{ py: 4 }}>
        <CircularProgress size={18} />
        <Typography color="text.secondary">Reading the record…</Typography>
      </Stack>
    );
  }

  if (error) {
    return (
      <Box>
        <ErrorNotice error={error} title="No track record" />
        <Button sx={{ mt: 2 }} variant="outlined" onClick={() => void reload()}>
          Try again
        </Button>
      </Box>
    );
  }

  if (!record) return null;

  const formats = Object.keys(record.win.by_format).sort();
  const populationsWithRows = new Set(record.coverage.rows.map((row) => row.population));

  return (
    <Box>
      <Typography variant="h6" sx={{ mb: 0.5 }}>
        Track record
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Every prediction the Lab issued, scored against the match once it is imported. Computed on
        read as of {record.today}: an import moves a prediction to scored by itself. This is a
        diagnostic over {record.total} prediction{record.total === 1 ? '' : 's'}, not evidence —
        every number carries its n, no threshold is set, and <code>make evaluate</code> remains
        where a choice-facing number comes from. The ranges are the simulator&apos;s as served.
      </Typography>

      <Stack direction="row" spacing={1} flexWrap="wrap" sx={{ mb: 2 }} data-testid="state-counts">
        {PREDICTION_STATES.map((state) => (
          <Chip
            key={state}
            size="small"
            variant="outlined"
            label={`${STATE_LABELS[state]} ${record.states[state] ?? 0}`}
            data-testid={`state-count-${state}`}
          />
        ))}
        <Chip size="small" variant="outlined" label={`total ${record.total}`} />
      </Stack>

      {harnessError && (
        <Alert severity="info" sx={{ mb: 2 }}>
          {harnessError.message}: the harness columns are empty. The record itself is complete.
        </Alert>
      )}

      <Paper variant="outlined" sx={{ mb: 3 }}>
        <Typography variant="subtitle1" fontWeight={600} sx={{ px: 2, pt: 1.5 }}>
          Win probability, scored
        </Typography>
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ display: 'block', px: 2, pb: 1 }}
        >
          The served headline probability against the result, over every scored prediction. The base
          rate is these rows&apos; own team1 win rate. The harness&apos;s columns are the display
          model&apos;s Brier and the training base rate&apos;s from{' '}
          <code>xi_evaluate_report.json</code>
          {harnessGeneratedAt ? ` (generated ${harnessGeneratedAt.slice(0, 19)})` : ''}, labelled by
          the window they came from.
        </Typography>
        <TableContainer>
          <Table size="small" aria-label="win probability scores">
            <TableHead>
              <TableRow>
                <TableCell>Population</TableCell>
                <TableCell align="right">n</TableCell>
                <TableCell align="right">
                  <MetricLabel metricKey="brier" label="Brier" />
                </TableCell>
                <TableCell align="right">
                  <MetricLabel metricKey="record_base_rate_brier" label="Base-rate Brier" />
                </TableCell>
                <TableCell align="right">Harness Brier (display)</TableCell>
                <TableCell align="right">Harness base-rate Brier</TableCell>
                <TableCell>Harness window</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              <WinRow label="All scored predictions" score={record.win.overall} />
              {formats.map((format) => (
                <WinRow
                  key={format}
                  label={format}
                  score={record.win.by_format[format]}
                  harness={harnessByFormat[format]}
                />
              ))}
            </TableBody>
          </Table>
        </TableContainer>
        <Box sx={{ px: 2, py: 2 }}>
          <TrackRecordReliabilityPlot
            bins={record.win.reliability}
            binCount={record.win.reliability_bins}
            n={record.win.overall.n}
          />
        </Box>
      </Paper>

      <Paper variant="outlined" sx={{ mb: 3 }}>
        <Typography variant="subtitle1" fontWeight={600} sx={{ px: 2, pt: 1.5 }}>
          Served 10–90 totals, coverage by innings
        </Typography>
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ display: 'block', px: 2, pb: 1 }}
        >
          Whether each actual innings total fell inside the range the Lab served, by the innings
          actually played. Factored and factorless simulators are different populations and are
          never pooled (B-12); an answer stored before the store recorded its simulator is
          &quot;unknown&quot;. No widening and no day/night adjustment: B-11 is open and the record
          shows it. The harness columns are the same format&apos;s coverage from the report, pooled
          over its folds, and over the folds with a shared factor where some had none.
        </Typography>
        <Stack
          direction="row"
          spacing={1}
          flexWrap="wrap"
          sx={{ px: 2, pb: 1 }}
          data-testid="population-counts"
        >
          {SIMULATOR_POPULATIONS.map((population) => (
            <Chip
              key={population}
              size="small"
              variant="outlined"
              label={`${POPULATION_LABELS[population]} ${record.coverage.populations[population] ?? 0}`}
              data-testid={`population-count-${population}`}
            />
          ))}
        </Stack>
        <TableContainer>
          <Table size="small" aria-label="coverage by population">
            <TableHead>
              <TableRow>
                <TableCell>Format</TableCell>
                <TableCell>Simulator population</TableCell>
                <TableCell align="right">n</TableCell>
                <TableCell align="right">
                  <MetricLabel metricKey="coverage_80" label="First innings in range" />
                </TableCell>
                <TableCell align="right">
                  <MetricLabel metricKey="coverage_80" label="Chase in range" />
                </TableCell>
                <TableCell align="right">Harness first innings</TableCell>
                <TableCell align="right">Harness chase</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {record.coverage.rows.length === 0 && (
                <TableRow>
                  <TableCell colSpan={7}>
                    <Typography variant="body2" color="text.secondary">
                      No scored prediction with a served range yet.
                    </Typography>
                  </TableCell>
                </TableRow>
              )}
              {record.coverage.rows.map((row) => {
                const harness = harnessByFormat[row.format];
                const factored = row.population === 'with_shared_factor';
                const first =
                  factored && harness?.firstInningsCoverageWithFactor != null
                    ? harness.firstInningsCoverageWithFactor
                    : harness?.firstInningsCoverage;
                const chase =
                  factored && harness?.chaseCoverageWithFactor != null
                    ? harness.chaseCoverageWithFactor
                    : harness?.chaseCoverage;
                return (
                  <TableRow
                    key={`${row.format}-${row.population}`}
                    hover
                    data-testid="coverage-row"
                  >
                    <TableCell>{row.format}</TableCell>
                    <TableCell>{POPULATION_LABELS[row.population]}</TableCell>
                    <TableCell align="right">{row.n_predictions}</TableCell>
                    <TableCell align="right">{coverage(row.first_innings)}</TableCell>
                    <TableCell align="right">{coverage(row.chase)}</TableCell>
                    <TableCell align="right">{share(first)}</TableCell>
                    <TableCell align="right">
                      {share(chase)}
                      {harness && (
                        <Typography variant="caption" color="text.secondary" display="block">
                          {harness.source}
                          {factored && harness.foldsWithoutFactor > 0
                            ? ', folds with a shared factor'
                            : ''}
                        </Typography>
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </TableContainer>
        {populationsWithRows.size > 1 && (
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ display: 'block', px: 2, py: 1 }}
          >
            {populationsWithRows.size} simulator populations on the record, each with its own
            denominator; there is no pooled coverage figure.
          </Typography>
        )}
      </Paper>

      <Paper variant="outlined" sx={{ mb: 3, px: 2, py: 1.5 }}>
        <Typography variant="subtitle1" fontWeight={600}>
          <MetricLabel
            metricKey="eleven_overlap"
            label="Elevens as named against elevens as fielded"
          />
        </Typography>
        <Typography variant="body2" data-testid="elevens-summary">
          {record.elevens.n === 0
            ? 'No scored prediction yet (n=0).'
            : `Over ${record.elevens.n} scored prediction${record.elevens.n === 1 ? '' : 's'}: mean ${stat(record.elevens.mean_overlap, 1)} of the named players took the field (min ${record.elevens.min_overlap}, max ${record.elevens.max_overlap}); ${record.elevens.complete} had every named player play.`}
        </Typography>
        <Typography variant="caption" color="text.secondary">
          Reported beside the scores and never used to exclude a prediction: a forecast for a side
          that did not play is a forecast of a different match, and the record says so rather than
          hiding it.
        </Typography>
      </Paper>

      <Paper variant="outlined" sx={{ mb: 3 }}>
        <Typography variant="subtitle1" fontWeight={600} sx={{ px: 2, pt: 1.5, pb: 1 }}>
          Every prediction, newest first
        </Typography>
        {record.predictions.length === 0 ? (
          <Typography variant="body2" color="text.secondary" sx={{ px: 2, pb: 2 }}>
            Nothing on the record yet. Every answer the Team Lab serves is stored and will appear
            here.
          </Typography>
        ) : (
          <TrackRecordList predictions={record.predictions} />
        )}
      </Paper>
    </Box>
  );
};

export default TrackRecordTab;
