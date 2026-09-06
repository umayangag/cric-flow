import React from 'react';
import {
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  List,
  ListItemButton,
  ListItemText,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import ErrorNotice from './common/ErrorNotice';
import { useCandidatePool } from '../hooks/useCandidatePool';
import type { PlayPlayer } from '../hooks/usePlayMode';
import type { PoolCandidate } from '../types';

/**
 * Pick one player out of a side's candidates — the "in" half of an add or a swap (P1-2).
 *
 * It is the same candidate list the pool picker reads (`GET /api/options/candidates`), so
 * the players offered here are the players the prediction would have chosen from, and a
 * player the retirement ledger excludes is on it, marked: an exclusion is a reason to
 * think twice, not a reason to hide a name from someone who knows better.
 */

export type PlayerPickerDialogProps = {
  open: boolean;
  onClose: () => void;
  format: string;
  clubId: number | null;
  teamName: string;
  matchDate: string;
  /** Whose place is being taken, where this is a swap rather than an addition. */
  replacing: PlayPlayer | null;
  /** Players already in the eleven; they are shown as already picked, not offered. */
  selectedIds: number[];
  onPick: (player: PlayPlayer) => void;
};

function matches(candidate: PoolCandidate, search: string): boolean {
  if (search.trim() === '') return true;
  return candidate.player_name.toLowerCase().includes(search.trim().toLowerCase());
}

const PlayerPickerDialog: React.FC<PlayerPickerDialogProps> = ({
  open,
  onClose,
  format,
  clubId,
  teamName,
  matchDate,
  replacing,
  selectedIds,
  onPick,
}) => {
  const [search, setSearch] = React.useState('');
  // The all-time list, because Play mode is where a user asks "what if he played?" about
  // someone the recency window has no opinion on. The pool a *prediction* uses is still
  // the one the Lab's pool controls say (D-12); this list only decides who can be picked.
  const pool = useCandidatePool({ format, clubId, matchDate, allTime: true, enabled: open });
  const held = new Set(selectedIds);
  const candidates = (pool.data?.candidates ?? []).filter((candidate) =>
    matches(candidate, search),
  );

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {replacing ? `Replace ${replacing.player_name}` : `Add a player to ${teamName}`}
      </DialogTitle>
      <DialogContent dividers>
        <Stack spacing={1} sx={{ mb: 2 }}>
          <Typography variant="body2" color="text.secondary">
            Picking a player re-scores the fixture straight away, and the change is shown against
            the eleven you had before it.
          </Typography>
          <TextField
            size="small"
            label="Search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          <ErrorNotice error={pool.error} title="Candidate list" />
        </Stack>
        {pool.loading ? (
          <Stack alignItems="center" sx={{ py: 4 }}>
            <CircularProgress size={24} />
          </Stack>
        ) : (
          <List dense>
            {candidates.map((candidate) => (
              <ListItemButton
                key={candidate.player_id}
                disabled={held.has(candidate.player_id)}
                onClick={() => {
                  onPick({
                    player_id: candidate.player_id,
                    player_name: candidate.player_name,
                  });
                  onClose();
                }}
              >
                <ListItemText
                  primary={`${candidate.player_name}${candidate.is_wicket_keeper ? ' (wk)' : ''}`}
                  secondary={
                    held.has(candidate.player_id)
                      ? 'already in this eleven'
                      : `last played ${candidate.last_played ?? '—'}`
                  }
                />
                {candidate.excluded && (
                  <Chip size="small" variant="outlined" color="warning" label={candidate.reason} />
                )}
              </ListItemButton>
            ))}
          </List>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
      </DialogActions>
    </Dialog>
  );
};

export default PlayerPickerDialog;
