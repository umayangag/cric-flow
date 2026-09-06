import React from 'react';
import {
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import ErrorNotice from './common/ErrorNotice';
import { useCandidatePool, useManualSelection } from '../hooks/useCandidatePool';
import type { PoolCandidate } from '../types';

/**
 * Pick a side's candidate pool by hand (D-12, step 2).
 *
 * It is optional and off by default: closing it without ticking anything leaves the
 * default pool and the unchanged flow. What it adds is the ability to *look* — the list
 * carries each player's last-played date, and every player the retirement ledger removed
 * is on it, struck through with the reason and an undo beside it, because an exclusion a
 * user cannot see is one they cannot reverse.
 */

/** What the list says about one excluded player. */
function exclusionLabel(candidate: PoolCandidate): string {
  if (candidate.reason === 'retired') return candidate.detail || 'recorded as retired';
  return 'you flagged him retired; nothing has corroborated it';
}

export type CandidatePoolDialogProps = {
  open: boolean;
  onClose: () => void;
  format: string;
  clubId: number | null;
  teamName: string;
  matchDate: string;
  /** Whether the list is drawn from the all-time pool rather than the recency window. */
  allTime: boolean;
  onAllTimeChange: (allTime: boolean) => void;
  /** The ids currently chosen for this side, or null while no manual pick is in force. */
  selected: number[] | null;
  onApply: (players: number[] | null) => void;
};

const CandidatePoolDialog: React.FC<CandidatePoolDialogProps> = ({
  open,
  onClose,
  format,
  clubId,
  teamName,
  matchDate,
  allTime,
  onAllTimeChange,
  selected,
  onApply,
}) => {
  const pool = useCandidatePool({ format, clubId, matchDate, allTime, enabled: open });
  const draft = useManualSelection();

  // The dialog opens on whatever is already chosen, so reopening it does not silently
  // discard a pick the user made a moment ago.
  React.useEffect(() => {
    if (!open) return;
    if (selected) draft.selectAll(selected);
    else draft.clear();
    // Only the opening seeds the draft; ticking afterwards is the user's.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const candidates = pool.data?.candidates ?? [];
  const summary = pool.data?.pool ?? null;
  const chosen = new Set(draft.selected ?? []);

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>Candidates for {teamName}</DialogTitle>
      <DialogContent dividers>
        <Stack spacing={1} sx={{ mb: 2 }}>
          <Typography variant="body2" color="text.secondary">
            Tick the players available for this match. Tick nothing and the default pool is used,
            unchanged.
          </Typography>
          {summary && (
            <Typography variant="caption" color="text.secondary">
              {summary.source === 'all_time'
                ? `everyone who has ever played for ${teamName}`
                : `played for ${teamName} in the last ${summary.window_months ?? 0} months`}
              {` — ${summary.size} players, ${summary.retired_excluded} excluded as retired`}
            </Typography>
          )}
          <FormControlLabel
            control={
              <Switch
                size="small"
                checked={allTime}
                onChange={(event) => onAllTimeChange(event.target.checked)}
              />
            }
            label="Show everyone who has ever played for this team"
          />
          <ErrorNotice error={pool.error} title="Candidate list" />
        </Stack>

        {pool.loading ? (
          <Stack alignItems="center" sx={{ py: 4 }}>
            <CircularProgress size={24} />
          </Stack>
        ) : (
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell padding="checkbox" />
                <TableCell>Player</TableCell>
                <TableCell>Last played</TableCell>
                <TableCell>Status</TableCell>
                <TableCell align="right">Retirement</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {candidates.map((candidate) => (
                <TableRow key={candidate.player_id} hover>
                  <TableCell padding="checkbox">
                    <Checkbox
                      size="small"
                      inputProps={{ 'aria-label': `Pick ${candidate.player_name}` }}
                      checked={chosen.has(candidate.player_id)}
                      onChange={(event) => draft.toggle(candidate.player_id, event.target.checked)}
                    />
                  </TableCell>
                  <TableCell
                    sx={candidate.excluded ? { textDecoration: 'line-through' } : undefined}
                  >
                    {candidate.player_name}
                    {candidate.is_wicket_keeper ? ' (wk)' : ''}
                    {/* The id is here so it can be typed into the Lab's must-include
                        field, which takes ids and not names — two players share a name
                        often enough that a name would not say which. */}
                    <Typography variant="caption" color="text.secondary" sx={{ ml: 1 }}>
                      #{candidate.player_id}
                    </Typography>
                  </TableCell>
                  <TableCell>{candidate.last_played ?? '—'}</TableCell>
                  <TableCell>
                    {candidate.excluded ? (
                      <Tooltip title={exclusionLabel(candidate)}>
                        <Chip size="small" variant="outlined" label={candidate.reason} />
                      </Tooltip>
                    ) : null}
                  </TableCell>
                  <TableCell align="right">
                    {candidate.excluded ? (
                      <Button
                        size="small"
                        disabled={pool.working}
                        onClick={() => void pool.undoExclusion(candidate.player_id)}
                      >
                        Undo
                      </Button>
                    ) : (
                      <Button
                        size="small"
                        disabled={pool.working}
                        onClick={() => void pool.flagRetired(candidate.player_id)}
                      >
                        Retired
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </DialogContent>
      <DialogActions>
        <Button
          onClick={() => {
            onApply(null);
            onClose();
          }}
        >
          Use the default pool
        </Button>
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="contained"
          disabled={!draft.selected?.length}
          onClick={() => {
            onApply(draft.selected);
            onClose();
          }}
        >
          Use these {draft.selected?.length ?? 0} players
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default CandidatePoolDialog;
