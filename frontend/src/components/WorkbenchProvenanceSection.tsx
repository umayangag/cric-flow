import React from 'react';
import {
  Alert,
  Box,
  Chip,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import SectionCard from './common/SectionCard';
import ErrorNotice from './common/ErrorNotice';
import type { ApiError } from '../lib/apiError';
import { MISSING, formatBytes, formatCount, formatWhen, shortDigest } from '../utils/format';
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
 * The newest export any model was trained from, or null when none records one.
 *
 * A model can be trained on the dataset that is still live and *still* be out of step
 * with its siblings: two exports of the same dataset, hours apart, produce different
 * training rows. Comparing each model against the newest export the set knows about
 * needs no new data — the models carry it — and catches the case a dataset digest
 * cannot (W5-1).
 */
export function newestExportAmong(models: MLModelStat[]): string | null {
  let newest: string | null = null;
  for (const model of models) {
    const exported = model.provenance?.exported_at;
    if (!exported) continue;
    if (newest === null || exported > newest) newest = exported;
  }
  return newest;
}

/** Whether this model came from an older export than the newest one on the box. */
export function isBehindNewestExport(model: MLModelStat, newestExport: string | null): boolean {
  const exported = model.provenance?.exported_at;
  return Boolean(newestExport && exported && exported < newestExport);
}

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
  const newestExport = newestExportAmong(models);
  const behind = models.filter((m) => isBehindNewestExport(m, newestExport));

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
                <> · {formatCount(live.dataset_match_files)} match files</>
              )}
              {live.dataset_extracted_at && (
                <> · extracted {formatWhen(live.dataset_extracted_at)}</>
              )}
            </Typography>
          ) : (
            <Typography variant="body2" color="text.secondary">
              The data directory holds no dataset manifest, so nothing can be compared against it.
              Run <strong>Import</strong> from Ops → Pipeline to acquire one and start recording
              provenance.
            </Typography>
          )}

          {stale.length > 0 && (
            <Alert severity="warning">
              {stale.length} model{stale.length === 1 ? ' was' : 's were'} trained on a different
              dataset from the one now on the box. Predictions from{' '}
              {stale.length === 1 ? 'it' : 'them'} reflect the older data until you re-train.
            </Alert>
          )}

          {behind.length > 0 && (
            <Alert severity="info">
              {behind.length} model{behind.length === 1 ? ' was' : 's were'} trained from an older
              export than {newestExport ? formatWhen(newestExport) : 'the newest one'}. The dataset
              may be the same; the training rows built from it are not, so these models and their
              siblings are not comparable.
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
                    <TableCell>Algorithm</TableCell>
                    <TableCell>Dataset</TableCell>
                    <TableCell>Cutoff</TableCell>
                    <TableCell>Trained</TableCell>
                    <TableCell>Exported</TableCell>
                    <TableCell align="right">Size</TableCell>
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
                        <TableCell>{model.algorithm ?? MISSING}</TableCell>
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
                          {formatWhen(provenance?.training_cutoff)}
                        </TableCell>
                        <TableCell sx={{ whiteSpace: 'nowrap' }}>
                          {formatWhen(model.trained_at ?? model.modified)}
                        </TableCell>
                        <TableCell sx={{ whiteSpace: 'nowrap' }}>
                          {formatWhen(provenance?.exported_at)}
                          {isBehindNewestExport(model, newestExport) && (
                            <Tooltip title="Trained from an older export than the newest one on this box.">
                              <Chip size="small" variant="outlined" label="behind" sx={{ ml: 1 }} />
                            </Tooltip>
                          )}
                        </TableCell>
                        <TableCell align="right">{formatBytes(model.size_bytes)}</TableCell>
                        <TableCell>{model.accuracy_display ?? MISSING}</TableCell>
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
