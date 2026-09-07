import React from 'react';
import { Alert, AlertTitle, Box, Chip, Stack, Typography } from '@mui/material';
import TeamTable from './TeamTable';
import MatchScorecard from './MatchScorecard';
import PoolSummary from './PoolSummary';
import type {
  PredictMustIncludeStatus,
  PredictSelectionSummary,
  PredictTeamSelectionResponse,
} from '../types';

/**
 * The answer: both elevens, the probability with its source, the simulated totals with the
 * ranges the draws produced, the scorecard, and the pool each XI was chosen out of.
 *
 * Every number here is rendered from the response and none is computed on top of it. Where
 * selection is rating-ordered (T20 and TEST, § 8.8) or the caller built the eleven (Play
 * mode) the notice is shown with the reason the response carried, and the tables carry no
 * marginal values, because nothing was maximised.
 */

export type TeamLabResultProps = {
  result: PredictTeamSelectionResponse;
  /** Widen one side's pool to all-time and predict again. */
  onWiden: (side: 1 | 2) => void;
};

/** What each selection state calls the elevens under it. */
const SELECTION_HEADINGS: Record<PredictSelectionSummary['objective'], string> = {
  win: 'Best 11 for each team',
  ratings: 'Rating-ordered 11 for each team',
  fixed: 'Your 11 for each team',
};

/**
 * What the notice is titled where nothing was optimised: the two states are different
 * facts, and one title for both would make a hand-built eleven read as a policy decision.
 */
const NOT_OPTIMISED_TITLES: Record<PredictSelectionSummary['objective'], string> = {
  win: 'Not optimised',
  ratings: 'Not optimised: picked by rating',
  fixed: 'Not optimised: your eleven',
};

/**
 * What the notice says when the response carried no reason. It is not a reason — the
 * surface will not invent one for the stack — it is the statement that none was served,
 * which is the only honest sentence available (§8.7).
 */
export const NO_REASON_SERVED =
  'The answer carried no reason for this. The eleven was not searched for, and this surface will not say why on the stack’s behalf.';

/**
 * Where the selection was not searched, say so at the XI, with the served reason (P1-4).
 *
 * A notice rather than a tooltip, because the whole point of the rating-ordered eleven is
 * that a reader should not act on its order without knowing what the order is — and the
 * probability above it is labelled, beside the number, as a read of that eleven and not a
 * search's result.
 */
const NotOptimisedNotice: React.FC<{ selection: PredictSelectionSummary }> = ({ selection }) => (
  <Alert severity="info" sx={{ mb: 2 }} data-testid="not-optimised-notice">
    <AlertTitle>{NOT_OPTIMISED_TITLES[selection.objective]}</AlertTitle>
    {selection.note ?? NO_REASON_SERVED}
  </Alert>
);

/**
 * What became of one side's must-include ids (P1-4, enforced since B-10).
 *
 * The lock is real now: go-app sends the ids to `/xi/optimize` as `must_include`, so an
 * ordinary answer holds every one of them and this reads "2 of 2 in the eleven". The
 * left-out chips are kept as the postcondition — if one ever appears, a lock the stack
 * accepted was not honoured, and the user sees that rather than a silently different
 * eleven. A request no eleven can satisfy is refused before it gets here.
 */
const MustIncludeOutcome: React.FC<{ status: PredictMustIncludeStatus; teamName: string }> = ({
  status,
  teamName,
}) => {
  if (status.requested === 0) return null;
  const held = status.requested - status.missing.length;
  return (
    <Stack
      direction="row"
      spacing={1}
      useFlexGap
      flexWrap="wrap"
      alignItems="center"
      sx={{ mb: 1 }}
    >
      <Chip
        size="small"
        variant="outlined"
        color={status.missing.length === 0 ? 'default' : 'warning'}
        label={`Must-include for ${teamName}: ${held} of ${status.requested} in the eleven`}
      />
      {status.missing.map((player) => (
        <Chip
          key={player.player_id}
          size="small"
          color="warning"
          label={`${player.player_name} left out`}
          title="You asked for this player and the selection was required to pick him, and did not. That is a defect in the selection, not a choice it made."
        />
      ))}
    </Stack>
  );
};

const TeamLabResult: React.FC<TeamLabResultProps> = ({ result, onWiden }) => (
  <Box sx={{ mt: 3 }}>
    <MatchScorecard
      scorecard={result.scorecard}
      winProbability={result.win_probability}
      forecast={result.forecast}
      selection={result.selection}
      toss={result.toss}
      served={{ ratings_through: result.ratings_through, run_id: result.run_id }}
      record={result.record}
      team1={result.team1_side.display_name}
      team2={result.team2_side.display_name}
    />
    {!result.selection.optimised && <NotOptimisedNotice selection={result.selection} />}
    <Typography variant="subtitle1" sx={{ mb: 1, fontWeight: 600 }}>
      {SELECTION_HEADINGS[result.selection.objective]}
    </Typography>
    <Stack direction={{ xs: 'column', md: 'row' }} spacing={3}>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <PoolSummary
          pool={result.team1_pool}
          teamName={result.team1_side.display_name}
          onWiden={() => onWiden(1)}
        />
        {result.selection.must_include && (
          <MustIncludeOutcome
            status={result.selection.must_include.team1}
            teamName={result.team1_side.display_name}
          />
        )}
        <TeamTable
          teamName={result.team1_side.display_name}
          players={result.team1}
          selection={result.selection}
        />
      </Box>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <PoolSummary
          pool={result.team2_pool}
          teamName={result.team2_side.display_name}
          onWiden={() => onWiden(2)}
        />
        {result.selection.must_include && (
          <MustIncludeOutcome
            status={result.selection.must_include.team2}
            teamName={result.team2_side.display_name}
          />
        )}
        <TeamTable
          teamName={result.team2_side.display_name}
          players={result.team2}
          selection={result.selection}
        />
      </Box>
    </Stack>
  </Box>
);

export default TeamLabResult;
