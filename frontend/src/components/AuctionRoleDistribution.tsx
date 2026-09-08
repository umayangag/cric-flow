import React from 'react';
import { Alert, Box, Chip, Stack, Typography } from '@mui/material';
import { MetricLabel } from './common/MetricInfo';
import RatingsAsOf from './RatingsAsOf';
import type { AuctionDistribution, AuctionRolesBlock, AuctionSlots } from '../types';

/**
 * What is left to buy, and what is left to fill (P3-1).
 *
 * Every number here is one the backend computed off the record the operator entered and
 * the two role predicates the served rating vectors support. Nothing is recomputed in the
 * browser — a count derived here would be a second definition of a number the surface is
 * meant to be reporting — and every labelled number opens its L-1 explainer, which is
 * where "batter" is defined as a label by elimination and where the overlap between the
 * keeper and bowling-option counts is stated.
 *
 * When the role read was refused the counts are gone rather than zero, and the refusal is
 * named: a zero would read as "there are no keepers left", which is a different and false
 * statement (§8.7).
 */

export type AuctionRoleDistributionProps = {
  slots: AuctionSlots;
  distribution?: AuctionDistribution;
  roles: AuctionRolesBlock;
};

/** The open-slot line, which needs no model, beside the constraints, which do. */
const OpenSlots: React.FC<{ slots: AuctionSlots }> = ({ slots }) => (
  <Box>
    <Typography variant="subtitle2" component="div">
      <MetricLabel
        metricKey="auction_open_slots"
        label="Open slots"
        value={`${slots.open} of ${slots.squad_size}`}
      />
    </Typography>
    <Typography variant="body2" color="text.secondary" data-testid="auction-open-slots">
      {slots.open} of {slots.squad_size} places open · {slots.filled} bought
    </Typography>
    {slots.by_role && (
      <Stack direction="row" spacing={1} sx={{ mt: 1 }} flexWrap="wrap" useFlexGap>
        <Chip
          size="small"
          data-testid="auction-keeper-slot"
          color={slots.by_role.keeper_needed ? 'warning' : 'default'}
          variant="outlined"
          label={
            slots.by_role.keeper_needed
              ? 'keeper still needed'
              : `keepers in the squad: ${slots.by_role.keepers}`
          }
        />
        <Chip
          size="small"
          data-testid="auction-bowling-slot"
          color={slots.by_role.bowling_options_short > 0 ? 'warning' : 'default'}
          variant="outlined"
          label={
            slots.by_role.bowling_options_short > 0
              ? `${slots.by_role.bowling_options_short} short of ${slots.by_role.min_bowlers} bowling options`
              : `bowling options: ${slots.by_role.bowling_options} of ${slots.by_role.min_bowlers}`
          }
        />
        {slots.by_role.unknown_roles > 0 && (
          <Chip
            size="small"
            variant="outlined"
            label={`${slots.by_role.unknown_roles} squad member(s) the served ratings have never seen`}
          />
        )}
      </Stack>
    )}
  </Box>
);

const AuctionRoleDistribution: React.FC<AuctionRoleDistributionProps> = ({
  slots,
  distribution,
  roles,
}) => (
  <Box sx={{ mb: 2 }} data-testid="auction-distribution">
    <OpenSlots slots={slots} />

    <Box sx={{ mt: 2 }}>
      <Typography variant="subtitle2" component="div">
        <MetricLabel
          metricKey="auction_available_by_role"
          label="Remaining pool by role"
          value={distribution ? `${distribution.available} available` : undefined}
        />
      </Typography>

      {!roles.available && (
        <Alert severity="warning" sx={{ mt: 1 }} data-testid="auction-roles-refused">
          The roles were not read, so no count off them is shown
          {roles.code ? ` — ${roles.code}` : ''}. {roles.message}
          {roles.hint ? ` ${roles.hint}` : ''} The list below is exactly as it was entered.
        </Alert>
      )}

      {roles.available && distribution && (
        <>
          <Stack direction="row" spacing={1} sx={{ mt: 1 }} flexWrap="wrap" useFlexGap>
            <Chip size="small" label={`available: ${distribution.available}`} />
            <Chip
              size="small"
              variant="outlined"
              data-testid="auction-available-keepers"
              label={`keepers: ${distribution.keepers}`}
            />
            <Chip
              size="small"
              variant="outlined"
              data-testid="auction-available-bowling-options"
              label={`bowling options: ${distribution.bowling_options}`}
            />
            <Chip
              size="small"
              variant="outlined"
              data-testid="auction-available-batters"
              label={`batters: ${distribution.batters}`}
            />
            {distribution.unknown > 0 && (
              <Chip
                size="small"
                variant="outlined"
                data-testid="auction-available-unknown"
                label={`unknown to the served ratings: ${distribution.unknown}`}
              />
            )}
          </Stack>
          <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 0.5 }}>
            Keepers and bowling options are the two predicates the model reads, so a keeper who also
            bowls is counted in both. A player who answers neither is a batter by elimination; one
            the served ratings have never seen is unknown, with no role invented for him.
          </Typography>
        </>
      )}

      {roles.available && roles.run_id && roles.ratings_through && (
        <Typography variant="caption" component="div" sx={{ mt: 0.5 }}>
          <RatingsAsOf served={{ ratings_through: roles.ratings_through, run_id: roles.run_id }} />
        </Typography>
      )}
    </Box>
  </Box>
);

export default AuctionRoleDistribution;
