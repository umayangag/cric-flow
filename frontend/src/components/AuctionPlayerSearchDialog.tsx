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
  List,
  ListItemButton,
  ListItemText,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import ErrorNotice from './common/ErrorNotice';
import { useAsync } from '../hooks/useAsync';
import { api } from '../api';
import type { PlayerSearchResult } from '../types';

/**
 * Find players to put on an auction list, by the name the auctioneer just called (P3-1).
 *
 * It is a cross-club search and not the candidate list: `GET /api/options/candidates` is
 * per club, because a prediction is about one side, and an auction room is not one side.
 * Nobody is filtered out — the retirement ledger's verdict is shown beside a name rather
 * than instead of one, because what the room is selling is not this system's to decide.
 *
 * `is_wicket_keeper` is shown as what it is: the *database's* name-set flag. It is not the
 * model's keeper role, which is read off the served rating vectors and appears on the
 * auction list once a player has been added.
 */

export type AuctionPlayerSearchDialogProps = {
  open: boolean;
  onClose: () => void;
  format: string;
  /** Players already on the list, so they are shown as listed and not offered twice. */
  listedIds: number[];
  onAdd: (playerIds: number[]) => void;
};

const AuctionPlayerSearchDialog: React.FC<AuctionPlayerSearchDialogProps> = ({
  open,
  onClose,
  format,
  listedIds,
  onAdd,
}) => {
  const [query, setQuery] = React.useState('');
  const [picked, setPicked] = React.useState<number[]>([]);
  const search = useAsync(api.searchPlayers, { errorMessage: 'The player search failed' });
  const { run: runSearch, reset } = search;

  React.useEffect(() => {
    if (!open) {
      reset();
      setPicked([]);
      setQuery('');
    }
  }, [open, reset, setPicked]);

  const listed = new Set(listedIds);
  const results = (search.data?.players ?? []) as PlayerSearchResult[];

  const toggle = (playerId: number) =>
    setPicked((current) =>
      current.includes(playerId) ? current.filter((id) => id !== playerId) : [...current, playerId],
    );

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>Add players to the list</DialogTitle>
      <DialogContent dividers>
        <Stack direction="row" spacing={1} sx={{ mb: 1 }}>
          <TextField
            size="small"
            fullWidth
            autoFocus
            label="Player name"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && query.trim().length >= 2) {
                void runSearch({ q: query.trim(), format });
              }
            }}
            helperText="At least two characters; it matches the start of the name or of any word in it."
          />
          <Button
            variant="contained"
            disabled={query.trim().length < 2 || search.loading}
            onClick={() => void runSearch({ q: query.trim(), format })}
          >
            Search
          </Button>
        </Stack>

        <ErrorNotice error={search.error} title="Search failed" />
        {search.loading && <CircularProgress size={20} />}

        <List dense>
          {results.map((player) => (
            <ListItemButton
              key={player.player_id}
              disabled={listed.has(player.player_id)}
              onClick={() => toggle(player.player_id)}
              data-testid={`search-result-${player.player_id}`}
            >
              <Checkbox
                edge="start"
                size="small"
                checked={picked.includes(player.player_id)}
                tabIndex={-1}
              />
              <ListItemText
                primary={
                  <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
                    <span>{player.player_name}</span>
                    {listed.has(player.player_id) && <Chip size="small" label="already listed" />}
                    {player.is_wicket_keeper && (
                      <Chip
                        size="small"
                        variant="outlined"
                        label="database flag: wicket-keeper"
                        title="player.is_wicket_keeper, set from an import's name sets. It is not the model's keeper role."
                      />
                    )}
                    {player.excluded && (
                      <Chip
                        size="small"
                        variant="outlined"
                        color="warning"
                        label={`ledger: ${player.reason ?? 'excluded'}`}
                      />
                    )}
                  </Stack>
                }
                secondary={
                  <>
                    {player.clubs.length > 0 ? player.clubs.join(', ') : 'no recorded club'}
                    {player.formats.length > 0 ? ` · ${player.formats.join(', ')}` : ''}
                    {player.last_played ? ` · last played ${player.last_played}` : ''}
                  </>
                }
              />
            </ListItemButton>
          ))}
        </List>
        {!search.loading && search.data && results.length === 0 && (
          <Typography variant="body2" color="text.secondary">
            No player of that name is in this database.
          </Typography>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="contained"
          disabled={picked.length === 0}
          onClick={() => {
            onAdd(picked);
            onClose();
          }}
        >
          Add {picked.length > 0 ? `(${picked.length})` : ''}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default AuctionPlayerSearchDialog;
