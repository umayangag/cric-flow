import React from 'react';
import { Box, Chip, Stack, Typography } from '@mui/material';
import { MetricLabel } from './common/MetricInfo';
import { formatProbabilityPoints, pointWithRange } from '../utils/format';
import type {
  PredictSelectionReason,
  PredictSelectionSummary,
  PredictTeamSelectedPlayer,
  SelectionRole,
} from '../types';

/**
 * P1-3 — the "why this player" card, and the rule it is built under.
 *
 * **The rule comes first: this card shows the inputs the selection actually consumed, and
 * only those.** Read from the code (ml/xi/optimizer.py, app/xi_service.py,
 * go-app/internal/services/predictteam), those inputs are:
 *
 * 1. **Marginal value** — `marginal_values`: P(win) for this eleven, minus P(win) with this
 *    player's rating vector neutralised, from the same objective model the search maximised
 *    and against the same opposing eleven. Present only where something was maximised.
 * 2. **Role in the eleven** — the two constraint predicates `_Pool` evaluates over the
 *    served as-of vectors: `is_keeper` (the keeper flag, which is also the objective's
 *    `has_keeper` feature) and `is_bowler` (expected balls bowled against the format
 *    threshold, which is also its `n_bowlers` feature).
 * 3. **Expected contribution** — L2-B's median and 10–90 range for runs and wickets in this
 *    fixture, off the same response.
 * 4. **Standing in the pool** — `rating_order_score`, the one composite `_greedy_seed`
 *    orders the pool by, and its percentile *within the pool as served*. It is the seed
 *    every search starts from and the whole answer where a format is not searched.
 * 5. **The next best** — the best excluded pool player, from exactly the single swaps
 *    `_best_neighbour` scores, under the same constraints and against the same opposing
 *    eleven, with the P(win) that swap costs.
 *
 * **What is deliberately not here, because the stack cannot produce it from a consumed
 * input** (each recorded in docs/PRODUCT_ROADMAP.md § 4): a *trajectory* — the served state
 * holds decayed accumulators as of one date, not a history, so no earlier value was read
 * and none can be shown; a *must-include* role — go-app puts a required id into the pool
 * and sends ml-service an empty `must_include`, so the search never treated anyone as
 * required; a *top-order anchor* role — batting position is read by the performance model,
 * never by the objective, so it explains the contribution and never the pick; an
 * *uncertainty on the gap* — one evaluation of the objective per candidate swap yields a
 * point estimate and nothing else, so the card says so instead of inventing an interval.
 *
 * Where the selection is rating-ordered (T20, TEST) the card says the simpler truth and
 * carries no marginal value and no next-best line, because nothing was maximised. Where the
 * caller built the eleven (Play mode) there is no selection to explain at all.
 */

export type WhyThisPlayerProps = {
  player: PredictTeamSelectedPlayer;
  /** How this eleven was chosen — which of the card's three states applies. */
  selection: PredictSelectionSummary;
};

/** How each role on the wire reads on the card. The vocabulary itself is the contract's. */
const ROLE_LABELS: Record<SelectionRole, string> = {
  keeper: 'Keeper',
  bowling_option: 'Bowling option',
};

const Field: React.FC<{
  metricKey: string;
  label: string;
  children: React.ReactNode;
}> = ({ metricKey, label, children }) => (
  <Box>
    <Typography variant="caption" color="text.secondary" component="div">
      <MetricLabel metricKey={metricKey} label={label} />
    </Typography>
    <Typography variant="body2" component="div">
      {children}
    </Typography>
  </Box>
);

/** The roles, or the true statement that the player answered neither constraint. */
const RoleChips: React.FC<{ roles: SelectionRole[] }> = ({ roles }) => (
  <Field metricKey="xi_role" label="Role in the eleven">
    {roles.length === 0 ? (
      'Neither a keeper nor a bowling option: picked on his rating alone.'
    ) : (
      <Stack direction="row" spacing={0.5} sx={{ flexWrap: 'wrap' }}>
        {roles.map((role) => (
          <Chip key={role} size="small" label={ROLE_LABELS[role]} />
        ))}
      </Stack>
    )}
  </Field>
);

/** Runs and wickets for this fixture, each with the band the model gave it. */
const ExpectedContribution: React.FC<{ player: PredictTeamSelectedPlayer }> = ({ player }) => (
  <Field metricKey="range_10_90" label="Expected contribution in this fixture">
    {pointWithRange(player.runs, player.runs_range)} runs,{' '}
    {pointWithRange(player.wickets, player.wickets_range, 1)} wickets
  </Field>
);

const Standing: React.FC<{ reason: PredictSelectionReason }> = ({ reason }) => (
  <Field metricKey="rating_percentile" label="Standing in the pool">
    Ahead of {reason.rating_percentile.toFixed(0)} % of the {reason.pool_size} candidates this
    eleven was chosen from.
  </Field>
);

/** The next-best line, or the reason the objective could not give one. */
const NextBest: React.FC<{ reason: PredictSelectionReason }> = ({ reason }) => {
  if (reason.best_alternative) {
    return (
      <Field metricKey="next_best_gap" label="Ahead of the next best">
        {formatProbabilityPoints(reason.best_alternative.win_probability_gap)} better than swapping
        in {reason.best_alternative.player_name}, the best replacement left in this pool. The
        objective gives a point estimate here; this computation yields no interval, so none is
        shown.
      </Field>
    );
  }
  if (reason.best_alternative_note) {
    return (
      <Field metricKey="next_best_gap" label="Ahead of the next best">
        {reason.best_alternative_note}
      </Field>
    );
  }
  return null;
};

/** The searched case: the objective's own reasons, headed by the marginal value. */
const OptimisedCard: React.FC<{
  player: PredictTeamSelectedPlayer;
  reason: PredictSelectionReason;
}> = ({ player, reason }) => (
  <Stack spacing={1.25}>
    <Field metricKey="marginal_value" label="What the eleven loses without him">
      {formatProbabilityPoints(player.marginal_value)} of win probability, if an average player took
      his place.
      {(player.marginal_value ?? 0) < 0 &&
        ' Negative: against this opposition the objective scores the eleven higher with that average player in his place.'}
    </Field>
    <RoleChips roles={reason.roles} />
    <ExpectedContribution player={player} />
    <Standing reason={reason} />
    <NextBest reason={reason} />
  </Stack>
);

/**
 * The rating-ordered case (T20, TEST): the simpler truth.
 *
 * No marginal value and no next-best line, because nothing was maximised — and a sentence
 * saying that the ordering on screen is not the win model's ranking, which is the half of
 * B-8 this surface can honestly close: rating order and the display model disagree about
 * who is better, so a swap for a higher-rated player can move the probability down.
 */
const RatingOrderedCard: React.FC<{
  player: PredictTeamSelectedPlayer;
  reason: PredictSelectionReason;
}> = ({ player, reason }) => (
  <Stack spacing={1.25}>
    <Typography variant="body2">
      <strong>Picked by rating, not by the win model.</strong> Nothing was maximised for this
      format, so there is no marginal value and no comparison with the next best.
    </Typography>
    <Field metricKey="selection_rating" label="Selection rating">
      {reason.selection_rating.toFixed(2)}
    </Field>
    <Standing reason={reason} />
    <RoleChips roles={reason.roles} />
    <ExpectedContribution player={player} />
    <Typography variant="caption" color="text.secondary">
      This ordering is the selection&apos;s own composite, and it is not the win model&apos;s
      ranking: the two disagree about who is better, so swapping in a higher-rated player can move
      the displayed probability down.
    </Typography>
  </Stack>
);

/** Play mode: the caller built this eleven, so there is no selection to explain. */
const HandBuiltCard: React.FC<{ player: PredictTeamSelectedPlayer }> = ({ player }) => (
  <Stack spacing={1.25}>
    <Typography variant="body2">
      You built this eleven, so nothing selected him and there is nothing to explain about the pick.
      What the models still say about him:
    </Typography>
    <ExpectedContribution player={player} />
  </Stack>
);

const WhyThisPlayer: React.FC<WhyThisPlayerProps> = ({ player, selection }) => {
  const reason = player.selection_reason;
  return (
    <Box sx={{ px: 2, py: 1.5 }}>
      <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
        Why {player.player_name}?
      </Typography>
      {!reason && <HandBuiltCard player={player} />}
      {reason && selection.optimised && <OptimisedCard player={player} reason={reason} />}
      {reason && !selection.optimised && <RatingOrderedCard player={player} reason={reason} />}
    </Box>
  );
};

export default WhyThisPlayer;
