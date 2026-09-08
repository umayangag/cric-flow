import React from 'react';
import { Button, Card, CardContent, MenuItem, Stack, TextField, Typography } from '@mui/material';
import ErrorNotice from './common/ErrorNotice';
import { useAsync } from '../hooks/useAsync';
import { api } from '../api';
import type { AuctionSummary, TeamSideOption } from '../types';

/**
 * Open an auction, or pick up one already on the record (P3-1).
 *
 * The formats offered are the simulator's — every later item of Phase 3 projects a total,
 * and a total needs an innings length — and the side is chosen by `club_id` rather than by
 * name, because a name is not a team (D-10).
 *
 * The squad size and the two constraints are the operator's: they are the *eleven's*
 * constraints, the same `min_bowlers` and `require_keeper` the predict path takes, so an
 * open slot is described in the objective's own vocabulary and not a second one.
 */

/** The formats the simulator serves, in the order the picker offers them. */
export const AUCTION_FORMATS = ['T20', 'T20I', 'ODI'] as const;

export type AuctionSetupProps = {
  auctions: AuctionSummary[];
  openAuctionId: string | null;
  onOpen: (auctionId: string) => void;
  onCreate: (body: {
    name: string;
    format: string;
    buyer_club_id: number;
    squad_size: number;
    min_bowlers: number;
    require_keeper: boolean;
  }) => void;
  working?: boolean;
};

const AuctionSetup: React.FC<AuctionSetupProps> = ({
  auctions,
  openAuctionId,
  onOpen,
  onCreate,
  working,
}) => {
  const [name, setName] = React.useState('');
  const [format, setFormat] = React.useState<string>(AUCTION_FORMATS[0]);
  const [clubId, setClubId] = React.useState<number | ''>('');
  const [squadSize, setSquadSize] = React.useState('25');
  const [minBowlers, setMinBowlers] = React.useState('5');

  const sides = useAsync(api.getTeamSidesByFormat, {
    errorMessage: 'Failed to load the sides for this format',
  });
  const { run: loadSides } = sides;
  React.useEffect(() => {
    void loadSides(format);
    setClubId('');
  }, [format, loadSides]);

  const options = (sides.data ?? []) as TeamSideOption[];
  const squadSizeIsWhole = /^\d+$/.test(squadSize) && Number.parseInt(squadSize, 10) > 0;

  return (
    <Card variant="outlined" sx={{ mb: 2 }}>
      <CardContent>
        <Typography variant="subtitle1" sx={{ mb: 1 }}>
          Auctions
        </Typography>

        {auctions.length > 0 && (
          <TextField
            select
            size="small"
            fullWidth
            label="Open an auction on the record"
            value={openAuctionId ?? ''}
            onChange={(event) => onOpen(event.target.value)}
            sx={{ mb: 2 }}
          >
            {auctions.map((auction) => (
              <MenuItem key={auction.id} value={auction.id}>
                {auction.name} — {auction.format}, {auction.buyer.name}
              </MenuItem>
            ))}
          </TextField>
        )}

        <Typography variant="subtitle2" sx={{ mb: 1 }}>
          Or start a new one
        </Typography>
        <ErrorNotice error={sides.error} title="Sides unavailable" />
        <Stack direction={{ xs: 'column', md: 'row' }} spacing={1} alignItems="flex-start">
          <TextField
            size="small"
            label="Name"
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
          <TextField
            select
            size="small"
            label="Format"
            value={format}
            onChange={(event) => setFormat(event.target.value)}
            sx={{ minWidth: 110 }}
          >
            {AUCTION_FORMATS.map((code) => (
              <MenuItem key={code} value={code}>
                {code}
              </MenuItem>
            ))}
          </TextField>
          <TextField
            select
            size="small"
            label="Buying side"
            value={clubId === '' ? '' : String(clubId)}
            onChange={(event) => setClubId(Number(event.target.value))}
            sx={{ minWidth: 220 }}
          >
            {options.map((side) => (
              <MenuItem key={side.club_id} value={String(side.club_id)}>
                {side.display_name}
              </MenuItem>
            ))}
          </TextField>
          <TextField
            size="small"
            label="Squad size"
            value={squadSize}
            onChange={(event) => setSquadSize(event.target.value)}
            sx={{ maxWidth: 120 }}
          />
          <TextField
            size="small"
            label="Min bowling options"
            value={minBowlers}
            onChange={(event) => setMinBowlers(event.target.value)}
            sx={{ maxWidth: 170 }}
          />
          <Button
            variant="contained"
            disabled={working || name.trim() === '' || clubId === '' || !squadSizeIsWhole}
            onClick={() =>
              onCreate({
                name: name.trim(),
                format,
                buyer_club_id: Number(clubId),
                squad_size: Number.parseInt(squadSize, 10),
                min_bowlers: Number.parseInt(minBowlers, 10) || 0,
                require_keeper: true,
              })
            }
          >
            Create
          </Button>
        </Stack>
        <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 1 }}>
          The formats offered are the ones the simulator serves. The squad size and the minimum
          bowling options are the eleven&apos;s own constraints — the same two the prediction path
          takes — so an open slot means the same thing on both surfaces.
        </Typography>
      </CardContent>
    </Card>
  );
};

export default AuctionSetup;
