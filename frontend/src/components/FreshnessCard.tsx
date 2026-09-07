import React from 'react';
import { Stack, Typography } from '@mui/material';
import SectionCard from './common/SectionCard';
import StatusPill from './common/StatusPill';
import { freshnessPillState, servedFreshnessLabel } from '../utils/opsStatusHelpers';
import type { OpsFreshness } from '../utils/opsStatusHelpers';

type Props = {
  freshness: OpsFreshness;
  formats: string[];
  formatsError?: string | null;
};

/**
 * Freshness, as one verdict with the facts that surround it.
 *
 * It replaced the "DB Data Freshness" grid, which bucketed each format's latest match at
 * 7 and 30 days and showed a worst-of badge — a second rule with a second threshold, which
 * on 2026-09-04 read *stale* (TEST's latest match was 8 days old) over a service that was
 * serving quite happily at 2 days of 14. The badge here is H-11's verdict and nothing
 * else, because H-11 is the rule a prediction is actually refused on; the per-format lag
 * is printed beside it as a date and a number of days, with no status of its own, because
 * a Test played a fortnight apart is not a fault (P2-1).
 */
const FreshnessCard: React.FC<Props> = ({ freshness, formats, formatsError }) => {
  const { served, database, retrain_due: retrainDue } = freshness;

  return (
    <SectionCard
      title="Freshness"
      subtitle={
        <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
          <Typography variant="body2" component="div">
            Served ratings
          </Typography>
          <StatusPill state={freshnessPillState(served.status)} label={served.status} />
          <Typography variant="body2" sx={{ opacity: 0.85 }}>
            {servedFreshnessLabel(served)}
          </Typography>
        </Stack>
      }
    >
      <Stack spacing={1}>
        <Typography variant="caption" fontWeight={600} color="text.secondary">
          Latest imported match, per format
        </Typography>
        {formatsError ? (
          <Typography variant="body2" color="error">
            {formatsError}
          </Typography>
        ) : formats.length === 0 ? (
          <Typography variant="body2" sx={{ opacity: 0.7 }}>
            Not available
          </Typography>
        ) : (
          formats.map((format) => {
            const lag = database[format];
            return (
              <Stack
                key={format}
                direction="row"
                alignItems="center"
                justifyContent="space-between"
              >
                <Typography variant="body2" sx={{ minWidth: 48 }}>
                  {format}
                </Typography>
                <Typography variant="body2" sx={{ opacity: 0.85 }}>
                  {lag?.latest_match_date
                    ? `${lag.latest_match_date} · ${lag.age_days ?? '?'}d ago · ${lag.match_count.toLocaleString()} matches`
                    : (lag?.note ?? 'not available')}
                </Typography>
              </Stack>
            );
          })
        )}
        <Typography variant="body2" data-testid="retrain-due">
          {retrainDue.status === 'retrain_due'
            ? `Retrain due: the database holds matches through ${retrainDue.latest_match_date}` +
              `${retrainDue.format ? ` (${retrainDue.format})` : ''}, ` +
              `${retrainDue.days_behind} days past the served ratings.`
            : retrainDue.status === 'up_to_date'
              ? `Retrain not due: the served ratings run through the latest imported match (${retrainDue.latest_match_date}).`
              : 'Retrain due: unknown — one of the two dates is missing, so the comparison was not made.'}
        </Typography>
      </Stack>
    </SectionCard>
  );
};

export default FreshnessCard;
