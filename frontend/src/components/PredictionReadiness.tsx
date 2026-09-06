import React from 'react';
import { Alert, AlertTitle, Typography } from '@mui/material';
import RatingsAsOf from './RatingsAsOf';
import { asObj } from '../utils/opsStatusHelpers';
import type { OpsStatus } from '../utils/opsStatusHelpers';

type Props = { status: OpsStatus | null };

/**
 * What this prediction is missing, said before it is run.
 *
 * The failure it guards has changed shape. It used to be zero-filled features: without
 * precompute there were no snapshots to read, so the vector was filled with zeros and
 * the models answered on them quite happily. P-6 deleted precompute, and the rating pass
 * reads `ball_event` directly, so that particular silence is gone.
 *
 * What is left are the two states where a prediction is refused rather than wrong, and
 * an operator should know which before clicking: nothing is loaded (no run, or one the
 * loader refused — D-6), and the loaded run's ratings are older than the configured
 * limit (H-11, `RATINGS_STALE`). Both come from the artifacts section the Ops tab
 * already renders, so this cannot say the box is ready while that says it is not. The
 * stale state carries its date through the same component every served prediction uses,
 * so the surface is dateless in no state (P1-4); the nothing-loaded state has no run and
 * so no date, and says that rather than showing one.
 */
const PredictionReadiness: React.FC<Props> = ({ status }) => {
  if (!status) return null;

  const artifacts = asObj(status.artifacts);
  const loadedRun = typeof artifacts.loaded_run === 'string' ? artifacts.loaded_run : '';
  const error = typeof artifacts.error === 'string' ? artifacts.error : '';
  const ratings = asObj(artifacts.ratings);
  const stale = loadedRun !== '' && ratings.fresh === false;

  if (loadedRun !== '' && !stale) return null;

  return (
    <Alert severity="warning" sx={{ mb: 2 }}>
      <AlertTitle>This prediction will not be answered</AlertTitle>
      {loadedRun === '' && (
        <Typography variant="body2">
          No training run is loaded, so there is no model to predict with and no ratings date to
          show.
          {error ? ` The artifacts on disk were refused: ${error}` : ''}
        </Typography>
      )}
      {stale && (
        <Typography variant="body2" component="div">
          The loaded run&apos;s ratings are older than the limit —{' '}
          <RatingsAsOf
            served={{
              ratings_through: String(ratings.ratings_through ?? 'an unknown date'),
              run_id: loadedRun,
            }}
          />{' '}
          — which is {String(ratings.age_days ?? '?')} days old against a limit of{' '}
          {String(ratings.max_age_days ?? '?')}. A live prediction is refused with{' '}
          <strong>RATINGS_STALE</strong> rather than answered from a squad that has moved on.
        </Typography>
      )}
      <Typography variant="body2" sx={{ mt: 1 }}>
        Run <strong>Retrain</strong> and then <strong>Reload</strong> from{' '}
        <strong>Ops → Pipeline</strong>.
      </Typography>
    </Alert>
  );
};

export default PredictionReadiness;
