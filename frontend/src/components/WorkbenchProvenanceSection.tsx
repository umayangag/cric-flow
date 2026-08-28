import React from 'react';
import { Box } from '@mui/material';
import Alert from '@mui/material/Alert';
import Chip from '@mui/material/Chip';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import SectionCard from './common/SectionCard';
import ErrorNotice from './common/ErrorNotice';
import type { ApiError } from '../lib/apiError';
import { formatWhen, shortDigest } from '../utils/datasetFormat';
import type { MLModelStat, ModelStatsResponse } from '../types';

type Props = {
  stats: ModelStatsResponse | null;
  loading?: boolean;
  error?: ApiError | string | null;
};

/** Whether a model's training data is current, stale, or genuinely unknown. */
type Freshness = 'live' | 'stale' | 'unknown';

export function freshnessOf(model: MLModelStat): Freshness {
  // Absent is a third state, not a synonym for stale: a model trained before
  // provenance existed is not out of date, it is unaccounted for — and flagging every
  // such model as stale would be noise rather than a warning.
  if (model.dataset_is_live === undefined) return 'unknown';
  return model.dataset_is_live ? 'live' : 'stale';
}

const FRESHNESS_LABEL: Record<Freshness, string> = {
  live: 'current',
  stale: 'stale',
  unknown: 'unknown',
};

const FRESHNESS_COLOUR: Record<Freshness, 'success' | 'warning' | 'default'> = {
  live: 'success',
  stale: 'warning',
  unknown: 'default',
};

const FRESHNESS_TOOLTIP: Record<Freshness, string> = {
  live: 'Trained on the dataset currently in the data directory.',
  stale:
    'Trained on a different dataset from the one now on the box. Re-train to use current data.',
  unknown:
    'This model records no dataset. It was trained before provenance existed, or from CSVs with no export manifest.',
};

/**
 * Which dataset produced which model (ops plan P-2).
 *
 * The plan opened by saying this was unanswerable. Every piece now exists: the extract
 * records a digest, the export carries it into a manifest, and training stamps it into
 * each model's sidecar. This is where it becomes an answer someone can read.
 */
const WorkbenchProvenanceSection: React.FC<Props> = ({ stats, loading, error }) => {
  const models = stats?.models ?? [];
  const live = stats?.live_dataset;
  const stale = models.filter((m) => freshnessOf(m) === 'stale');
  const unknown = models.filter((m) => freshnessOf(m) === 'unknown');

  return (
    <SectionCard
      title="Model provenance"
      subtitle="Which dataset produced which model, and whether that dataset is still the one on the box."
    >
      {loading && <Typography variant="body2">Loading model provenance…</Typography>}
      <ErrorNotice error={error} title="Could not load model provenance" />

      {!loading && !error && (
        <>
          {live ? (
            <Typography variant="body2" component="div">
              Live dataset:{' '}
              {live.dataset_feed && (
                <>
                  <strong>{live.dataset_feed}</strong> ·{' '}
                </>
              )}
              <Tooltip title={live.dataset_sha256 ?? ''}>
                <span>sha256 {shortDigest(live.dataset_sha256)}</span>
              </Tooltip>
              {live.dataset_match_files != null && (
                <> · {live.dataset_match_files.toLocaleString()} match files</>
              )}
              {live.dataset_extracted_at && (
                <> · extracted {formatWhen(live.dataset_extracted_at)}</>
              )}
            </Typography>
          ) : (
            <Typography variant="body2" color="text.secondary">
              The data directory holds no dataset manifest, so nothing can be compared against it.
              Fetch and extract a dataset from the Data tab to start recording provenance.
            </Typography>
          )}

          {stale.length > 0 && (
            <Alert severity="warning">
              {stale.length} model{stale.length === 1 ? ' was' : 's were'} trained on a different
              dataset from the one now on the box. Predictions from{' '}
              {stale.length === 1 ? 'it' : 'them'} reflect the older data until you re-train.
            </Alert>
          )}

          {models.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No models on disk.
            </Typography>
          ) : (
            <Box sx={{ overflowX: 'auto' }}>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>Model</TableCell>
                    <TableCell>Format</TableCell>
                    <TableCell>Dataset</TableCell>
                    <TableCell>Cutoff</TableCell>
                    <TableCell>Trained</TableCell>
                    <TableCell>Accuracy</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {models.map((model) => {
                    const freshness = freshnessOf(model);
                    const provenance = model.provenance;
                    return (
                      <TableRow key={`${model.model_name}-${model.match_format}`}>
                        <TableCell>{model.model_name}</TableCell>
                        <TableCell>{model.match_format}</TableCell>
                        <TableCell sx={{ whiteSpace: 'nowrap' }}>
                          <Tooltip title={FRESHNESS_TOOLTIP[freshness]}>
                            <Chip
                              size="small"
                              color={FRESHNESS_COLOUR[freshness]}
                              variant={freshness === 'unknown' ? 'outlined' : 'filled'}
                              label={FRESHNESS_LABEL[freshness]}
                            />
                          </Tooltip>
                          {provenance?.dataset_sha256 && (
                            <Tooltip title={provenance.dataset_sha256}>
                              <Typography variant="caption" fontFamily="monospace" sx={{ ml: 1 }}>
                                {shortDigest(provenance.dataset_sha256)}
                              </Typography>
                            </Tooltip>
                          )}
                        </TableCell>
                        {/* The cutoff varies per run and is recorded nowhere else: two
                            models from the same export with different cutoffs are
                            different models. */}
                        <TableCell sx={{ whiteSpace: 'nowrap' }}>
                          {provenance?.training_cutoff
                            ? formatWhen(provenance.training_cutoff)
                            : '—'}
                        </TableCell>
                        <TableCell sx={{ whiteSpace: 'nowrap' }}>
                          {formatWhen(model.trained_at ?? model.modified)}
                        </TableCell>
                        <TableCell>{model.accuracy_display ?? '—'}</TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </Box>
          )}

          {unknown.length > 0 && (
            <Typography variant="caption" color="text.secondary">
              {unknown.length} model{unknown.length === 1 ? '' : 's'} record no dataset. They
              predate provenance, or were trained from CSVs with no export manifest — which is not
              the same as being out of date.
            </Typography>
          )}
        </>
      )}
    </SectionCard>
  );
};

export default WorkbenchProvenanceSection;
