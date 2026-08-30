import React, { useMemo } from 'react';
import {
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
import { MISSING, formatWhen, shortDigest } from '../utils/format';
import type { MLModelStat, ModelStatsResponse } from '../types';

type Props = {
  /** The format that was evaluated. Models for other formats are irrelevant here. */
  format: string;
  stats: ModelStatsResponse | null;
  loading: boolean;
};

/** Models whose match_format is the evaluated one, in a stable order. */
function modelsForFormat(stats: ModelStatsResponse | null, format: string): MLModelStat[] {
  const wanted = format.trim().toUpperCase();
  if (!stats || !wanted) return [];
  return stats.models
    .filter((m) => (m.match_format || '').trim().toUpperCase() === wanted)
    .sort((a, b) => a.model_name.localeCompare(b.model_name));
}

/**
 * Which models answered this evaluation, and what they were trained on.
 *
 * Evaluation has no per-request choice of model — W0-2 established that and removed
 * the toggle pretending otherwise — so the models that answered are the ones
 * ml-service has loaded for the format. That is what makes this answerable at all,
 * and it is also its one caveat: if artifacts are reloaded between running an
 * evaluation and reading it, this describes what is loaded *now*. `trained_at` is
 * there so that is visible rather than assumed.
 *
 * The interesting column is the last one. A model trained on a dataset that is no
 * longer the one on this box has been scored against matches its training set may
 * already contain — which is the difference between a result and a number.
 */
const EvaluationProvenance: React.FC<Props> = ({ format, stats, loading }) => {
  const models = useMemo(() => modelsForFormat(stats, format), [stats, format]);

  if (loading) {
    return (
      <Typography variant="body2" color="text.secondary" sx={{ fontStyle: 'italic' }}>
        Loading model provenance…
      </Typography>
    );
  }

  if (models.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No provenance recorded for {format || 'this format'}. Models trained before provenance
        existed do not carry it — that is unknown, not clean.
      </Typography>
    );
  }

  return (
    <Box>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
        Produced by the artifacts loaded for <strong>{format}</strong>. Evaluation has no
        per-request model choice, so these are the models that answered.
      </Typography>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell>Model</TableCell>
            <TableCell>Trained at</TableCell>
            <TableCell>Training cutoff</TableCell>
            <TableCell>Dataset</TableCell>
            <TableCell>Still the live dataset?</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {models.map((model) => (
            <TableRow key={model.model_name}>
              <TableCell>{model.model_name}</TableCell>
              <TableCell>{formatWhen(model.trained_at ?? model.modified)}</TableCell>
              <TableCell>{formatWhen(model.provenance?.training_cutoff)}</TableCell>
              <TableCell>
                {model.provenance?.dataset_sha256 ? (
                  <Tooltip title={model.provenance.dataset_sha256}>
                    <span>{shortDigest(model.provenance.dataset_sha256)}</span>
                  </Tooltip>
                ) : (
                  MISSING
                )}
              </TableCell>
              <TableCell>
                <LiveVerdict isLive={model.dataset_is_live} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Box>
  );
};

/**
 * Three states, not two.
 *
 * "We cannot tell" is a real answer — a model with no recorded dataset, or a box whose
 * data directory has no manifest — and rendering it as "no" would assert staleness on
 * no evidence.
 */
const LiveVerdict: React.FC<{ isLive?: boolean }> = ({ isLive }) => {
  if (isLive === undefined) {
    return <Chip size="small" variant="outlined" label="unknown" />;
  }
  return isLive ? (
    <Chip size="small" color="success" label="yes" />
  ) : (
    <Tooltip title="This model was trained on a dataset that is no longer on this box. It may have been trained on matches it is now being scored against.">
      <Chip size="small" color="warning" label="no — trained on other data" />
    </Tooltip>
  );
};

export default EvaluationProvenance;
