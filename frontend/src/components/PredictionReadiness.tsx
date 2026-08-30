import React from 'react';
import { Alert, AlertTitle, Typography } from '@mui/material';
import { derivePipelineSteps } from '../utils/pipelineSteps';
import type { OpsStatus } from '../utils/opsStatusHelpers';

type Props = { status: OpsStatus | null };

/**
 * What this prediction will be missing, said before it is run.
 *
 * Prediction has real preconditions, and a missing one does not fail — it produces a
 * confident-looking number computed on zeros. Sequence and windowed features come from
 * `feature_raw_stats_snapshots`, and when precompute has not run there is nothing to
 * read, so the feature vector is filled with zeros and the models predict on them
 * quite happily. That is the failure the plan calls out: not an error, an answer
 * nobody can tell is worthless.
 *
 * The check is the pipeline state the Ops tab already derives, so this cannot say the
 * pipeline is ready while the Ops tab says it is not.
 */
const PredictionReadiness: React.FC<Props> = ({ status }) => {
  if (!status) return null;

  const steps = derivePipelineSteps(status);
  const incomplete = ['import', 'precompute', 'export'].filter(
    (id) => steps.find((s) => s.id === id)?.status !== 'success',
  );
  if (incomplete.length === 0) return null;

  const precomputeMissing = incomplete.includes('precompute');

  return (
    <Alert severity="warning" sx={{ mb: 2 }}>
      <AlertTitle>This prediction will be running on incomplete data</AlertTitle>
      <Typography variant="body2">
        These pipeline steps have not completed on this box:{' '}
        <strong>{incomplete.join(', ')}</strong>.
      </Typography>
      {precomputeMissing && (
        <Typography variant="body2" sx={{ mt: 1 }}>
          Without precompute there are no feature snapshots to read, so form and windowed statistics
          are <strong>filled with zeros</strong> rather than left out. The models will still answer,
          and the answer will not be worth anything.
        </Typography>
      )}
      <Typography variant="body2" sx={{ mt: 1 }}>
        Run them from <strong>Ops → Pipeline</strong>.
      </Typography>
    </Alert>
  );
};

export default PredictionReadiness;
