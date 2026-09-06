import React from 'react';
import { Alert, AlertTitle, Box, Stack, Typography } from '@mui/material';
import TeamTable from './TeamTable';
import MatchScorecard from './MatchScorecard';
import PoolSummary from './PoolSummary';
import type { PredictTeamSelectionResponse } from '../types';

/**
 * The answer: both elevens, the probability with its source, the simulated totals with the
 * ranges the draws produced, the scorecard, and the pool each XI was chosen out of.
 *
 * Every number here is rendered from the response and none is computed on top of it. Where
 * selection is rating-ordered (T20 and TEST, § 8.8) the notice is shown and the tables carry
 * no marginal values, because nothing was maximised.
 */

export type TeamLabResultProps = {
  result: PredictTeamSelectionResponse;
  /** Widen one side's pool to all-time and predict again. */
  onWiden: (side: 1 | 2) => void;
};

const TeamLabResult: React.FC<TeamLabResultProps> = ({ result, onWiden }) => (
  <Box sx={{ mt: 3 }}>
    <MatchScorecard
      scorecard={result.scorecard}
      winProbability={result.win_probability}
      toss={result.toss}
      served={{ ratings_through: result.ratings_through, run_id: result.run_id }}
      team1={result.team1_side.display_name}
      team2={result.team2_side.display_name}
    />
    {!result.selection.optimised && (
      <Alert severity="info" sx={{ mb: 2 }}>
        <AlertTitle>Not optimised</AlertTitle>
        {result.selection.note ??
          'This format has no win objective that ranks, so the eleven is picked by as-of rating under the same constraints.'}
      </Alert>
    )}
    <Typography variant="subtitle1" sx={{ mb: 1, fontWeight: 600 }}>
      {result.selection.optimised ? 'Best 11 for each team' : 'Rating-ordered 11 for each team'}
    </Typography>
    <Stack direction={{ xs: 'column', md: 'row' }} spacing={3}>
      <Box sx={{ flex: 1 }}>
        <PoolSummary
          pool={result.team1_pool}
          teamName={result.team1_side.display_name}
          onWiden={() => onWiden(1)}
        />
        <TeamTable
          teamName={result.team1_side.display_name}
          players={result.team1}
          optimised={result.selection.optimised}
        />
      </Box>
      <Box sx={{ flex: 1 }}>
        <PoolSummary
          pool={result.team2_pool}
          teamName={result.team2_side.display_name}
          onWiden={() => onWiden(2)}
        />
        <TeamTable
          teamName={result.team2_side.display_name}
          players={result.team2}
          optimised={result.selection.optimised}
        />
      </Box>
    </Stack>
  </Box>
);

export default TeamLabResult;
