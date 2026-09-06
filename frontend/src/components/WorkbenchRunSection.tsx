import React from 'react';
import {
  Alert,
  Chip,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import SectionCard from './common/SectionCard';
import KeyValueList from './common/KeyValueList';
import JsonCollapse from './common/JsonCollapse';
import { MetricLabel } from './common/MetricInfo';
import { formatMetricValue } from '../utils/format';
import type { ApiError } from '../lib/apiError';
import type { XiStatusResponse } from '../types';

type Props = {
  status: XiStatusResponse | null;
  loading: boolean;
  error: ApiError | null;
};

/**
 * The run's own headline metrics, one row per format, each explained by its key.
 *
 * They used to be a JSON dump, which is honest and unreadable: `objective_auc` means
 * nothing to a reader who has not lived inside the plan. The keys are the service's
 * (`ml/xi/glossary.py`), so every one of them opens the same explainer the evaluation
 * tab uses.
 */
const HeadlineMetrics: React.FC<{ metrics: Record<string, Record<string, number>> }> = ({
  metrics,
}) => {
  const keys = Array.from(
    new Set(Object.values(metrics).flatMap((byMetric) => Object.keys(byMetric ?? {}))),
  );
  if (!keys.length) return null;

  return (
    <Table size="small" aria-label="run headline metrics">
      <TableHead>
        <TableRow>
          <TableCell>Format</TableCell>
          {keys.map((key) => (
            <TableCell key={key} align="right">
              <MetricLabel metricKey={key} label={key} />
            </TableCell>
          ))}
        </TableRow>
      </TableHead>
      <TableBody>
        {Object.entries(metrics).map(([format, byMetric]) => (
          <TableRow key={format} hover>
            <TableCell>{format}</TableCell>
            {keys.map((key) => (
              <TableCell key={key} align="right">
                {byMetric?.[key] == null ? '—' : formatMetricValue(byMetric[key])}
              </TableCell>
            ))}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
};

/**
 * Why a format has no headline metrics, in the manifest's own words (B-3).
 *
 * A run at today's cutoff trains every format and has nothing to score them on. Showing
 * the metrics table alone would leave that as a blank, which reads the same as a run that
 * trained nothing — so the manifest's reason is rendered beside the numbers rather than
 * left in the training log.
 */
const FormatNotes: React.FC<{ notes: Record<string, string> }> = ({ notes }) => {
  const entries = Object.entries(notes);
  if (!entries.length) return null;

  return (
    <Alert severity="info" aria-label="why a format has no headline metrics">
      {entries.map(([format, note]) => (
        <Typography key={format} variant="body2">
          <strong>{format}</strong>: {note}
        </Typography>
      ))}
    </Alert>
  );
};

/**
 * What the loaded model *is*: the run that produced it, and what that run recorded
 * about itself (H-16).
 *
 * It replaced two sections that read the deleted win model's feature list and a
 * per-artifact provenance sidecar. Both answered "which data produced this model?" by
 * inference; the manifest answers it by record — the dataset digest, the cutoff, the
 * commit, the hyperparameters the grid chose and the run's own headline metrics, written
 * beside the artifacts they describe.
 */
const WorkbenchRunSection: React.FC<Props> = ({ status, loading, error }) => (
  <SectionCard
    title="Loaded run"
    subtitle="The run `current` points at, and what its manifest records (H-16)."
  >
    {loading && <Typography variant="body2">Loading…</Typography>}
    {error && <Alert severity="error">{error.message}</Alert>}
    {!loading && !error && status && !status.loaded && (
      <Alert severity="warning">
        {status.error
          ? `The artifacts on disk were refused: ${status.error}`
          : 'No run is loaded. Run Retrain and then Reload from Ops → Pipeline.'}
      </Alert>
    )}
    {!loading && !error && status?.loaded && (
      <Stack spacing={1.5}>
        <KeyValueList
          items={[
            { label: 'Run id', value: status.run_id ?? '—' },
            { label: 'Trained through (cutoff)', value: status.manifest?.cutoff ?? '—' },
            { label: 'Ratings through', value: status.ratings_through ?? '—' },
            { label: 'Dataset sha', value: status.manifest?.dataset_sha?.slice(0, 16) ?? '—' },
            { label: 'Commit', value: status.manifest?.git_sha?.slice(0, 7) ?? '—' },
            { label: 'Players rated', value: String(status.players) },
          ]}
        />
        <Stack direction="row" spacing={0.5} flexWrap="wrap">
          {status.formats.map((f) => (
            <Chip key={f} size="small" label={f} />
          ))}
        </Stack>
        {status.manifest?.hyperparameters && (
          <JsonCollapse
            data={status.manifest.hyperparameters}
            summary="Show the hyperparameters the grid chose, and why"
          />
        )}
        {status.manifest?.metrics && <HeadlineMetrics metrics={status.manifest.metrics} />}
        {status.manifest?.format_notes && <FormatNotes notes={status.manifest.format_notes} />}
      </Stack>
    )}
  </SectionCard>
);

export default WorkbenchRunSection;
