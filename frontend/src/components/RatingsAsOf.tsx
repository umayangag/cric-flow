import React from 'react';
import { Typography } from '@mui/material';
import type { PredictServedRatings } from '../types';

/**
 * "Ratings as of <date> · run <id>": the one component that says which rating state a
 * number was computed from (P1-5, P1-4).
 *
 * It is one component rather than a sentence each surface writes for itself because the
 * date is the Lab's honesty claim in miniature: a prediction is "as of last import" and
 * says so beside every number. Two surfaces spelling that differently — or one of them
 * forgetting it — is how a dateless number gets copied out of the Lab. The served state
 * comes off the payload the number came from, never off a status poll, which describes
 * whatever is loaded at the moment of the poll.
 *
 * The title is UI copy about a label, not metric prose: the date and the run are not
 * numbers the L-1 glossary explains, so there is no explainer to open here.
 */
export const RATINGS_AS_OF_TITLE =
  'The served ratings include every match through this date and none after it; the run is the ' +
  'model build they were loaded from. A prediction past the freshness limit is refused, never served stale.';

export type RatingsAsOfProps = {
  /** The run and the date its ratings run through, off the payload being described. */
  served: PredictServedRatings;
};

const RatingsAsOf: React.FC<RatingsAsOfProps> = ({ served }) => (
  <Typography
    variant="body2"
    color="text.secondary"
    component="span"
    title={RATINGS_AS_OF_TITLE}
    data-testid="ratings-as-of"
  >
    ratings as of <strong>{served.ratings_through}</strong> · run {served.run_id}
  </Typography>
);

export default RatingsAsOf;
