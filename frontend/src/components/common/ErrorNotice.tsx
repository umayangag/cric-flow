import React from 'react';
import { Alert, AlertTitle, Box, Typography } from '@mui/material';
import { ApiError } from '../../lib/apiError';

/**
 * A remedy for a failure the app knows by name.
 *
 * The backend's own `hint` is always shown; this adds the part it cannot know, which
 * is where in *this* UI to go and fix it. Codes with no entry are not a gap — the hint
 * alone is usually enough, and inventing advice for an unknown code would be worse
 * than saying nothing.
 */
const REMEDIES: Record<string, string> = {
  MODEL_NOT_LOADED:
    'Train and load artifacts for this format: Ops → Pipeline → Export, then the train step for the model named above.',
  MISSING_FORMAT:
    'Choose a format before running. Models are per-format; there is no pooled model.',
  CONTRIBUTIONS_CSV_MISSING:
    'Produce backtest_contributions.csv first: Evaluate → Export contributions. Ops → Train Combination Meta needs it.',
  NO_DATA: 'The dataset directory is empty. Ops → Import acquires and loads a dataset.',
  FORMAT_NOT_FOUND:
    'No matches have been imported for this format. Ops → Import, then Precompute, then Export.',
  NO_SQUAD:
    'No player pool for this team in this format — the teams are derived from imported matches, so a team with no history here cannot be picked from.',
  RATINGS_STALE:
    'Ops → Pipeline: run Retrain, then Reload. The Lab answers again once the loaded run’s ratings are inside the limit; until then no prediction is served from the old ones.',
};

export type ErrorNoticeProps = {
  /** Nothing is rendered when this is null, so callers need no conditional of their own. */
  error: ApiError | string | null | undefined;
  /** Overrides the default heading when the surface has a better name for the failure. */
  title?: string;
  /** Extra context the caller knows and the error does not, e.g. which panel failed. */
  context?: string;
};

/**
 * The one way this app shows a failed request.
 *
 * Every surface used to render its own red box over `error.message`, which is why the
 * `hint` the backend writes specifically to be shown was never shown: it was not in
 * the string. Three things are laid out as what they are rather than concatenated —
 * what went wrong, what to do about it, and what *is* available when the failure is
 * that something named was not.
 */
const ErrorNotice: React.FC<ErrorNoticeProps> = ({ error, title, context }) => {
  if (!error) return null;
  const apiError = typeof error === 'string' ? new ApiError(error) : error;
  const remedy = apiError.code ? REMEDIES[apiError.code] : undefined;

  return (
    <Alert severity="error" role="alert">
      <AlertTitle>{title ?? 'Request failed'}</AlertTitle>
      <Typography variant="body2">{apiError.message}</Typography>

      {context && (
        <Typography variant="body2" sx={{ mt: 0.5, opacity: 0.85 }}>
          {context}
        </Typography>
      )}

      {apiError.hint && (
        <Typography variant="body2" sx={{ mt: 1 }}>
          <strong>Next:</strong> {apiError.hint}
        </Typography>
      )}

      {remedy && remedy !== apiError.hint && (
        <Typography variant="body2" sx={{ mt: 0.5 }}>
          {remedy}
        </Typography>
      )}

      {apiError.available && apiError.available.length > 0 && (
        <Box sx={{ mt: 1 }}>
          <Typography variant="body2">
            <strong>Available:</strong> {apiError.available.join(', ')}
          </Typography>
        </Box>
      )}

      {apiError.code && (
        <Typography variant="caption" sx={{ display: 'block', mt: 1, opacity: 0.7 }}>
          {apiError.code}
        </Typography>
      )}
    </Alert>
  );
};

export default ErrorNotice;
