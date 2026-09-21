import React from 'react';
import { Alert, AlertTitle, Typography } from '@mui/material';
import RatingsAsOf from './RatingsAsOf';
import { asObj, readFreshness } from '../utils/opsStatusHelpers';
import { RATINGS_STALE_CODE } from '../types';
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
 * limit (H-11, `RATINGS_STALE`). Both are read off the one freshness object go-app
 * assembles (P2-1), which carries ml-service's verdict copied through — the same
 * computation the prediction itself is refused on, so this notice cannot say the box is
 * ready while the request would be refused, nor differ from the Ops badge. The stale
 * state carries its date through the same component every served prediction uses, so the
 * surface is dateless in no state (P1-4); the nothing-loaded state has no run and so no
 * date, and says that rather than showing one.
 */
const PredictionReadiness: React.FC<Props> = ({ status }) => {
  if (!status) return null;

  const served = readFreshness(status).served;
  const artifacts = asObj(status.artifacts);
  const loadedRun = typeof artifacts.loaded_run === 'string' ? artifacts.loaded_run : '';
  const error = typeof artifacts.error === 'string' ? artifacts.error : '';

  if (served.status === 'fresh') return null;
  const stale = served.status === 'stale';
  const unknown = served.status === 'unknown';

  return (
    <Alert severity="warning" sx={{ mb: 2 }}>
      <AlertTitle>
        {unknown ? 'This prediction may not be answered' : 'This prediction will not be answered'}
      </AlertTitle>
      {unknown && (
        <Typography variant="body2">
          The ML service did not answer, so there is no verdict on the ratings and no date to show.
          Whether a prediction would be served is unknown.
        </Typography>
      )}
      {served.status === 'not_loaded' && (
        <Typography variant="body2">
          No training run is loaded, so there is no model to predict with and no ratings date to
          show.
          {error ? ` The artifacts on disk were refused: ${error}` : ''}
        </Typography>
      )}
      {stale && (
        <Typography variant="body2" component="div">
          The loaded run&apos;s data was built to {served.data_through ?? 'an unknown date'}, which
          is {served.data_age_days ?? '?'} days ago against a limit of {served.max_age_days ?? '?'}.
          Its last match is{' '}
          <RatingsAsOf
            served={{
              ratings_through: served.ratings_through ?? 'an unknown date',
              run_id: loadedRun,
            }}
          />
          . A live prediction is refused with <strong>{served.code ?? RATINGS_STALE_CODE}</strong>{' '}
          rather than answered from a squad that has moved on.
        </Typography>
      )}
      {!unknown && (
        <Typography variant="body2" sx={{ mt: 1 }}>
          Run <strong>Retrain</strong> and then <strong>Reload</strong> from{' '}
          <strong>Ops → Pipeline</strong>.
        </Typography>
      )}
    </Alert>
  );
};

export default PredictionReadiness;
