import React from 'react';
import { Box, Button, Chip, Link, Stack, Tooltip, Typography } from '@mui/material';
import type { PoolExcludedCandidate, PoolSummary as PoolSummaryDTO } from '../types';

/**
 * Say which candidates an XI was chosen out of.
 *
 * The pool used to be all-time and unstated, so an XI could contain a player who retired
 * a decade ago and the surface gave the reader nothing to notice it by (D-12). Every
 * number here comes off the wire — the window the backend applied, the size it produced,
 * and each player the retirement ledger removed — because a filter that is not shown is
 * indistinguishable from no filter at all (§8.7).
 */

/** How the pool line reads for each source the backend can report. */
function poolSentence(pool: PoolSummaryDTO, teamName: string): string {
  if (pool.source === 'manual') {
    return `pool: ${pool.size} players you chose for ${teamName}`;
  }
  if (pool.source === 'all_time') {
    return `pool: everyone who has ever played for ${teamName} (${pool.size} players)`;
  }
  const window = pool.window_months ?? 0;
  return `pool: played for ${teamName} in the last ${window} months (${pool.size} players)`;
}

/** The exclusion count, spelled only when there is one. */
function exclusionSentence(pool: PoolSummaryDTO): string {
  if (pool.retired_excluded <= 0) return '';
  const players = pool.retired_excluded === 1 ? 'player' : 'players';
  return `, ${pool.retired_excluded} ${players} excluded as retired`;
}

/** Why one candidate was left out, in the words a person can act on. */
function exclusionReason(excluded: PoolExcludedCandidate): string {
  const detail = excluded.detail ? ` — ${excluded.detail}` : '';
  if (excluded.reason === 'retired') return `recorded as retired${detail}`;
  return 'you flagged him retired; nothing has corroborated it';
}

export type PoolSummaryProps = {
  pool: PoolSummaryDTO;
  teamName: string;
  /** Widen this side's pool to all-time and predict again. Absent while none is offered. */
  onWiden?: () => void;
  /** Put one excluded player back, by withdrawing the flag that removed him. */
  onUndoExclusion?: (playerId: number) => void;
  /** The player an undo is in flight for, so the button cannot be pressed twice. */
  undoingPlayerId?: number | null;
};

const PoolSummary: React.FC<PoolSummaryProps> = ({
  pool,
  teamName,
  onWiden,
  onUndoExclusion,
  undoingPlayerId,
}) => (
  <Box sx={{ mb: 1 }}>
    <Typography variant="caption" color="text.secondary" component="div">
      {poolSentence(pool, teamName)}
      {exclusionSentence(pool)}
      {pool.source === 'recency_window' && onWiden && (
        <>
          {' · '}
          <Link component="button" type="button" variant="caption" onClick={onWiden}>
            use the all-time pool
          </Link>
        </>
      )}
    </Typography>
    {pool.excluded?.length ? (
      <Stack spacing={0.5} sx={{ mt: 0.5 }}>
        {pool.excluded.map((excluded) => (
          <Stack
            key={excluded.player_id}
            direction="row"
            spacing={1}
            alignItems="center"
            flexWrap="wrap"
          >
            <Tooltip title={exclusionReason(excluded)}>
              <Typography
                variant="caption"
                sx={{ textDecoration: 'line-through' }}
                color="text.disabled"
              >
                {excluded.player_name}
              </Typography>
            </Tooltip>
            <Chip size="small" variant="outlined" label={exclusionReason(excluded)} />
            {excluded.last_played && (
              <Typography variant="caption" color="text.secondary">
                last played {excluded.last_played}
              </Typography>
            )}
            {onUndoExclusion && (
              <Button
                size="small"
                onClick={() => onUndoExclusion(excluded.player_id)}
                disabled={undoingPlayerId === excluded.player_id}
              >
                Undo
              </Button>
            )}
          </Stack>
        ))}
      </Stack>
    ) : null}
  </Box>
);

export default PoolSummary;
