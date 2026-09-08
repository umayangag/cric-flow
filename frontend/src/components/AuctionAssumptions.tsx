import React from 'react';
import {
  Alert,
  Box,
  Button,
  Chip,
  Divider,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import ErrorNotice from './common/ErrorNotice';
import type {
  AuctionListedPlayer,
  AuctionOppositionSuggestion,
  AuctionProjectionPlayer,
  AuctionRecord,
} from '../types';
import type { ApiError } from '../lib/apiError';

/**
 * The projection's three assumptions, named by the operator (P3-2).
 *
 * A projection of a candidate's output is conditional on the eleven he would join, the
 * opposition it would face and the grounds, and at an auction not one of those is a fact.
 * They are named here, held on the record, and carried back on every projection — so a
 * range on screen always says which guess it was computed for.
 *
 * The opposition is never a silently neutral side (§8.7). A side's last recorded eleven is
 * offered as a starting point, with the date of the match it came from, and the operator
 * edits it before saving. Nothing here assembles a plausible-looking eleven by rating:
 * that would be this surface inventing an opposition and presenting it as evidence.
 */

export type AuctionAssumptionsProps = {
  record: AuctionRecord;
  /** The auction's own list: the pool both elevens are named from. */
  listed: AuctionListedPlayer[];
  suggestion: AuctionOppositionSuggestion | null;
  suggesting: boolean;
  suggestionError: ApiError | null;
  onSuggest: (clubId: number) => void;
  onSave: (body: {
    likely_xi?: number[];
    opposition?: { club_id: number; player_ids: number[] };
  }) => void;
  saving: boolean;
  saveError: ApiError | null;
};

const TEAM_SIZE = 11;

/** A named eleven as chips, with each member removable while it is being edited. */
const NamedEleven: React.FC<{
  players: AuctionProjectionPlayer[];
  testId: string;
  onRemove?: (playerId: number) => void;
}> = ({ players, testId, onRemove }) => (
  <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap data-testid={testId}>
    {players.length === 0 && (
      <Typography variant="body2" color="text.secondary">
        Nobody named yet.
      </Typography>
    )}
    {players.map((player) => (
      <Chip
        key={player.player_id}
        size="small"
        variant="outlined"
        label={player.player_name}
        onDelete={onRemove ? () => onRemove(player.player_id) : undefined}
      />
    ))}
  </Stack>
);

const AuctionAssumptions: React.FC<AuctionAssumptionsProps> = ({
  record,
  listed,
  suggestion,
  suggesting,
  suggestionError,
  onSuggest,
  onSave,
  saving,
  saveError,
}) => {
  const [likelyXI, setLikelyXI] = React.useState<number[]>(
    record.likely_xi.map((player) => player.player_id),
  );
  const [clubId, setClubId] = React.useState('');
  const [opposition, setOpposition] = React.useState<AuctionProjectionPlayer[]>(
    record.opposition?.players ?? [],
  );
  const [oppositionClub, setOppositionClub] = React.useState<number | null>(
    record.opposition?.club_id ?? null,
  );

  // A suggestion arriving replaces the side being edited: the operator asked for that
  // team's last eleven, and merging it into whatever was there would produce a side
  // neither they nor the database named.
  React.useEffect(() => {
    if (!suggestion) return;
    setOpposition(suggestion.players);
    setOppositionClub(suggestion.club_id);
  }, [suggestion]);

  const nameByID = new Map(listed.map((player) => [player.player_id, player.player_name]));
  const likelyPlayers = likelyXI.map((id) => ({
    player_id: id,
    player_name: nameByID.get(id) ?? `player ${id}`,
  }));

  return (
    <Box sx={{ mb: 2 }} data-testid="auction-assumptions">
      <Typography variant="subtitle2">The projection’s assumptions</Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
        A projection is for an eleven, against an opposition, at grounds — all three named here,
        none of them a fact. Every projection carries them back, so a range on screen says which
        guess it was made for.
      </Typography>

      <ErrorNotice error={saveError} title="The assumptions were not saved" />

      <Stack spacing={2}>
        <Box>
          <Typography variant="body2" fontWeight={600}>
            The likely eleven ({likelyXI.length} of {TEAM_SIZE})
          </Typography>
          <Typography variant="caption" color="text.secondary" component="div">
            The buyer’s squad so far plus the operator’s guesses. Ten named leaves the eleventh
            place for the candidate; a side that is not eleven with him in it is refused, exactly as
            the predict path refuses one.
          </Typography>
          <TextField
            select
            size="small"
            fullWidth
            label="Add to the likely eleven"
            value=""
            sx={{ mt: 1 }}
            onChange={(event) => {
              const id = Number(event.target.value);
              setLikelyXI((current) =>
                current.includes(id) || current.length >= TEAM_SIZE ? current : [...current, id],
              );
            }}
          >
            {listed
              .filter((player) => !likelyXI.includes(player.player_id))
              .map((player) => (
                <MenuItem key={player.player_id} value={player.player_id}>
                  {player.player_name}
                </MenuItem>
              ))}
          </TextField>
          <Box sx={{ mt: 1 }}>
            <NamedEleven
              players={likelyPlayers}
              testId="auction-likely-xi"
              onRemove={(id) => setLikelyXI((current) => current.filter((held) => held !== id))}
            />
          </Box>
        </Box>

        <Divider />

        <Box>
          <Typography variant="body2" fontWeight={600}>
            The opposition ({opposition.length} of {TEAM_SIZE})
          </Typography>
          <Typography variant="caption" color="text.secondary" component="div">
            A league has no single opposition, so this is one the operator names. Start from a
            side’s last recorded eleven and edit it; a projection against no one is refused rather
            than made against a neutral side.
          </Typography>
          <Stack direction="row" spacing={1} sx={{ mt: 1 }} alignItems="center">
            <TextField
              size="small"
              label="Club id"
              value={clubId}
              onChange={(event) => setClubId(event.target.value)}
              sx={{ width: 140 }}
            />
            <Button
              size="small"
              variant="outlined"
              disabled={suggesting || !clubId}
              onClick={() => onSuggest(Number(clubId))}
            >
              Use their last eleven
            </Button>
          </Stack>
          <ErrorNotice error={suggestionError} title="No starting point for that side" />
          {suggestion && (
            <Alert severity="info" sx={{ mt: 1 }} data-testid="auction-opposition-suggestion">
              {suggestion.name}’s eleven from {suggestion.from_match.match_date}
              {suggestion.from_match.event_name
                ? ` (${suggestion.from_match.event_name})`
                : ''}. {suggestion.note}
            </Alert>
          )}
          <Box sx={{ mt: 1 }}>
            <NamedEleven
              players={opposition}
              testId="auction-opposition"
              onRemove={(id) =>
                setOpposition((current) => current.filter((player) => player.player_id !== id))
              }
            />
          </Box>
        </Box>

        <Box>
          <Typography variant="body2" fontWeight={600}>
            The grounds
          </Typography>
          <Stack direction="row" spacing={0.5} sx={{ mt: 0.5 }} flexWrap="wrap" useFlexGap>
            {record.venue_ids.length === 0 && (
              <Typography variant="body2" color="text.secondary" data-testid="auction-no-grounds">
                This auction names no grounds, so nothing can be projected: a projection is per
                ground.
              </Typography>
            )}
            {record.venue_ids.map((venueID) => (
              <Chip key={venueID} size="small" label={`venue ${venueID}`} />
            ))}
          </Stack>
        </Box>

        <Box>
          <Button
            variant="contained"
            size="small"
            disabled={saving}
            data-testid="auction-save-assumptions"
            onClick={() =>
              onSave({
                likely_xi: likelyXI,
                ...(oppositionClub !== null && opposition.length > 0
                  ? {
                      opposition: {
                        club_id: oppositionClub,
                        player_ids: opposition.map((player) => player.player_id),
                      },
                    }
                  : {}),
              })
            }
          >
            Save the assumptions
          </Button>
        </Box>
      </Stack>
    </Box>
  );
};

export default AuctionAssumptions;
