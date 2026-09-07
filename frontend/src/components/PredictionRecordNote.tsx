import React from 'react';
import { Typography } from '@mui/material';
import type { PredictRecordBlock } from '../types';

/**
 * Whether this answer went on the record, beside the date it was served from (P2-3).
 *
 * It sits next to `RatingsAsOf` because it answers the neighbouring question. That one says
 * which rating state the numbers came from; this one says whether the numbers will still be
 * here to be scored when the match has been played. A prediction that was served but not
 * filed is a hole in the track record, and the person reading the number is the only one
 * who can do anything about it — so the failure is shown here, in the answer, and not left
 * in a server log (§8.7).
 *
 * The stored case is deliberately quiet: it is the ordinary state, and the id is there for
 * someone who wants to look the answer up again, not something to shout about. The failed
 * case is loud, because it is a claim about the record being incomplete.
 */
export const PREDICTION_RECORD_TITLE =
  'Every prediction this Lab issues is stored exactly as it was served, so it can be scored ' +
  'against the result once the match has been played. This is the id it was stored under.';

export const PREDICTION_NOT_RECORDED_TITLE =
  'The prediction is unaffected — it was computed and served normally — but it was not stored, ' +
  'so it will not appear in the track record and cannot be scored later.';

export type PredictionRecordNoteProps = {
  /** The `record` block off the answer being described. */
  record: PredictRecordBlock;
};

const PredictionRecordNote: React.FC<PredictionRecordNoteProps> = ({ record }) => {
  if (!record.stored) {
    return (
      <Typography
        variant="body2"
        color="warning.main"
        component="span"
        title={PREDICTION_NOT_RECORDED_TITLE}
        data-testid="prediction-record"
      >
        not recorded: {record.reason || 'the store gave no reason'}
      </Typography>
    );
  }
  return (
    <Typography
      variant="body2"
      color="text.secondary"
      component="span"
      title={PREDICTION_RECORD_TITLE}
      data-testid="prediction-record"
    >
      recorded as <strong>{record.id}</strong>
    </Typography>
  );
};

export default PredictionRecordNote;
