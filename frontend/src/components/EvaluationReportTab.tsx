import React from 'react';
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Typography,
} from '@mui/material';
import ErrorNotice from './common/ErrorNotice';
import MetricSpectrumLegend from './common/MetricSpectrumLegend';
import EvaluationWalkForward from './EvaluationWalkForward';
import EvaluationSelectionMetrics from './EvaluationSelectionMetrics';
import EvaluationPerformanceTable from './EvaluationPerformance';
import EvaluationSimulation from './EvaluationSimulation';
import { useEvaluationReport } from '../hooks/useEvaluationReport';

/**
 * The evaluation tab: L4's report, and nothing else.
 *
 * It used to evaluate one match at a time against the batting, bowling and fielding models
 * — which is what those models could be scored on, and also why the numbers never added up
 * to a verdict about the system. The harness runs rolling origins over every format and
 * writes one file (`make evaluate`); this tab reads it. There is no form, because there
 * is no choice for the browser to make: the folds, the locked window and the seeds are the
 * harness's, and a cutoff chosen here would be a choice made against the locked window.
 */
const EvaluationReportTab: React.FC = () => {
  const { report, loading, error, reload, formats, format, setFormat, formatReport } =
    useEvaluationReport();

  if (loading && !report) {
    return (
      <Stack direction="row" spacing={1} alignItems="center" sx={{ py: 4 }}>
        <CircularProgress size={18} />
        <Typography color="text.secondary">Loading the evaluation report…</Typography>
      </Stack>
    );
  }

  if (error) {
    return (
      <Box>
        <ErrorNotice error={error} title="No evaluation report" />
        <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
          The report is written by <code>make evaluate</code>, which runs the walk-forward folds,
          the locked window and the train/serve parity check, and lands beside the artifacts.
        </Typography>
        <Button sx={{ mt: 2 }} variant="outlined" onClick={() => void reload()}>
          Try again
        </Button>
      </Box>
    );
  }

  if (!report) return null;

  return (
    <Box>
      <Typography variant="h6" sx={{ mb: 1 }}>
        Evaluation report
      </Typography>
      <Stack direction="row" spacing={1} flexWrap="wrap" sx={{ mb: 2 }}>
        <Chip
          size="small"
          variant="outlined"
          label={`generated ${report.generated_at.slice(0, 19)}`}
        />
        <Chip size="small" variant="outlined" label={`source ${report.source}`} />
        <Chip size="small" variant="outlined" label={`locked from ${report.locked_start}`} />
        {report.locked_window && (
          <Chip
            size="small"
            variant="outlined"
            label={`window rotated ${report.locked_window.rotated_on}, from ${report.locked_window.previous_start}`}
          />
        )}
        <Chip size="small" variant="outlined" label={`${report.n_rows.toLocaleString()} matches`} />
        <Chip
          size="small"
          variant="outlined"
          label={`${report.n_player_rows.toLocaleString()} player rows`}
        />
        <Chip size="small" variant="outlined" label={`seeds ${report.seeds.join(', ')}`} />
      </Stack>

      <Alert severity={report.serving_parity.passed ? 'success' : 'error'} sx={{ mb: 3 }}>
        {report.serving_parity.passed
          ? 'Train/serve parity holds (H-8): the last matches rebuilt through the as-of serving path match the training frame exactly.'
          : 'Train/serve parity FAILED (H-8): the serving path and the training frame disagree. Nothing served from these artifacts can be trusted until it does.'}
      </Alert>
      {report.gates && report.gates.passed === false && (
        <Alert severity="error" sx={{ mb: 3 }}>
          Gate registry FAILED (H-23): the report prints a gate without saying what it varies and
          what it holds fixed — {report.gates.problems?.join('; ')}.
        </Alert>
      )}
      {report.glossary && report.glossary.passed === false && (
        <Alert severity="warning" sx={{ mb: 3 }}>
          Metric glossary incomplete (L-1): this report prints a metric no entry explains, so it
          reaches these tables with nothing to say about itself —{' '}
          {report.glossary.problems?.join('; ')}.
        </Alert>
      )}

      <Stack direction="row" spacing={2} alignItems="center" sx={{ mb: 3 }}>
        <FormControl size="small" sx={{ minWidth: 140 }}>
          <InputLabel>Format</InputLabel>
          <Select value={format ?? ''} label="Format" onChange={(e) => setFormat(e.target.value)}>
            {formats.map((f) => (
              <MenuItem key={f} value={f}>
                {f}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <Button variant="outlined" size="small" onClick={() => void reload()} disabled={loading}>
          Reload
        </Button>
      </Stack>

      {formatReport && (
        <>
          <MetricSpectrumLegend />
          <EvaluationWalkForward report={formatReport} />
          <EvaluationSelectionMetrics report={formatReport} gates={report.gates?.registry} />
          <EvaluationPerformanceTable
            performance={formatReport.locked.performance}
            title="Performance, locked window"
            caption="Scored once per release on matches at or after the locked start, and never used for a choice (H-19)."
          />
          <EvaluationPerformanceTable
            performance={formatReport.walk_forward.summary.performance}
            title="Performance, mean over folds"
            caption="Every number is a mean over the rolling origins with its spread; differences inside the spread are not evidence (H-14)."
          />
          <EvaluationSimulation report={formatReport} />
        </>
      )}
    </Box>
  );
};

export default EvaluationReportTab;
