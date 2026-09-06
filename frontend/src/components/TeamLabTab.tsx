import React from 'react';
import { Box, Typography } from '@mui/material';
import ErrorNotice from './common/ErrorNotice';
import PredictionReadiness from './PredictionReadiness';
import CandidatePoolDialog from './CandidatePoolDialog';
import TeamLabInputs from './TeamLabInputs';
import TeamLabResult from './TeamLabResult';
import { useTeamLab, type SidePoolChoice } from '../hooks/useTeamLab';
import type { TeamSideOption } from '../types';

/**
 * The Team Lab: one surface on `POST /api/predict/team-selection` (P1-1).
 *
 * It is the Upcoming-match tab grown up rather than a second tab beside it, because two
 * surfaces on one endpoint drift and only one of them is ever right. The state and the
 * request live in {@link useTeamLab}; this component wires that state to three
 * presentational pieces — the inputs, the candidate list, and the answer — and computes
 * nothing of its own. Every number on screen is one the response carried.
 */
const TeamLabTab: React.FC = () => {
  const lab = useTeamLab();

  // Which side's candidate list is open, if any. Manual picking is optional and off by
  // default, so this starts closed and staying closed changes nothing (D-12).
  const [openPoolFor, setOpenPoolFor] = React.useState<1 | 2 | null>(null);

  const sides: {
    side: 1 | 2;
    team: TeamSideOption | null;
    pool: SidePoolChoice;
    setPool: React.Dispatch<React.SetStateAction<SidePoolChoice>>;
  }[] = [
    { side: 1, team: lab.team1, pool: lab.team1Pool, setPool: lab.setTeam1Pool },
    { side: 2, team: lab.team2, pool: lab.team2Pool, setPool: lab.setTeam2Pool },
  ];
  const openSide = sides.find((entry) => entry.side === openPoolFor) ?? null;

  return (
    <Box>
      <Typography variant="h6" sx={{ mb: 2 }}>
        Team Lab
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Pick a fixture, choose each side&apos;s candidate pool, say what the toss is and what an
        eleven must contain, then Optimise. Teams are listed one side at a time — India (men) and
        India (women) are different teams — and both sides of a fixture are the same. The date must
        be today or within {lab.maxFutureDays} days, which is as far ahead as the ratings are
        trusted.
      </Typography>

      <TeamLabInputs lab={lab} onOpenPool={setOpenPoolFor} />

      {openSide?.team && (
        <CandidatePoolDialog
          open
          onClose={() => setOpenPoolFor(null)}
          format={lab.format}
          clubId={openSide.team.club_id}
          teamName={openSide.team.display_name}
          matchDate={lab.matchDate}
          allTime={openSide.pool.allTime}
          onAllTimeChange={(allTime) => openSide.setPool((current) => ({ ...current, allTime }))}
          selected={openSide.pool.players}
          onApply={(players) => openSide.setPool((current) => ({ ...current, players }))}
        />
      )}

      <PredictionReadiness status={lab.opsStatus} />

      <Box sx={{ mb: 2 }}>
        <ErrorNotice error={lab.error} title="Prediction failed" />
      </Box>

      {lab.result && (
        <TeamLabResult result={lab.result} onWiden={(side) => void lab.widenPool(side)} />
      )}
    </Box>
  );
};

export default TeamLabTab;
