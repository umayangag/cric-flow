import React from 'react';
import {
  Box,
  Button,
  Chip,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import type { AuctionListedPlayer, AuctionPlayerRoles, AuctionPlayerState } from '../types';

/**
 * The auction list: every player, the state the operator last recorded, and what the model
 * says he is (P3-1).
 *
 * The role chips are the objective's two constraint predicates read off the served rating
 * vectors, and nothing else. "Batter" appears only where the model has seen the player and
 * neither predicate holds — a label by elimination, which the column header's explainer
 * says in as many words. A player the served state has never seen is "unknown"; no role is
 * invented for him (§8.7).
 *
 * There is no value, no marginal value and no win probability on this table, and there is
 * no "best XI" button: the module is valuation and projection, never XI-picking.
 */

export type AuctionPlayerListProps = {
  players: AuctionListedPlayer[];
  /** The buying side, so a row can say whether a sale was to this squad or a rival's. */
  buyerClubId: number;
  /** Record a sale for one player. Absent while the record is read-only. */
  onSell?: (player: AuctionListedPlayer) => void;
  onUnsold?: (player: AuctionListedPlayer) => void;
  onUndo?: (player: AuctionListedPlayer) => void;
  working?: boolean;
};

const STATE_LABELS: Record<AuctionPlayerState, string> = {
  available: 'available',
  sold: 'sold',
  unsold: 'unsold',
};

/** The role chips for one player, or the reason there are none. */
export const RoleChips: React.FC<{ roles?: AuctionPlayerRoles }> = ({ roles }) => {
  if (!roles) {
    return (
      <Typography variant="caption" color="text.secondary">
        not read
      </Typography>
    );
  }
  if (!roles.known) {
    return <Chip size="small" variant="outlined" label="unknown to the served ratings" />;
  }
  if (roles.roles.length === 0) {
    return <Chip size="small" variant="outlined" label="batter (by elimination)" />;
  }
  return (
    <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
      {roles.roles.map((role) => (
        <Chip key={role} size="small" variant="outlined" label={role.replace('_', ' ')} />
      ))}
    </Stack>
  );
};

const AuctionPlayerList: React.FC<AuctionPlayerListProps> = ({
  players,
  buyerClubId,
  onSell,
  onUnsold,
  onUndo,
  working,
}) => (
  <TableContainer>
    <Table size="small" aria-label="Auction list">
      <TableHead>
        <TableRow>
          <TableCell>Player</TableCell>
          <TableCell>State</TableCell>
          <TableCell>Buyer</TableCell>
          <TableCell align="right">Price</TableCell>
          <TableCell>Role</TableCell>
          <TableCell />
        </TableRow>
      </TableHead>
      <TableBody>
        {players.map((player) => (
          <TableRow key={player.player_id} hover data-testid={`auction-row-${player.player_id}`}>
            <TableCell>{player.player_name}</TableCell>
            <TableCell>
              <Chip
                size="small"
                data-testid={`auction-state-${player.player_id}`}
                color={player.state === 'sold' ? 'primary' : 'default'}
                variant={player.state === 'available' ? 'outlined' : 'filled'}
                label={STATE_LABELS[player.state]}
              />
            </TableCell>
            <TableCell>
              {player.buyer_name ? (
                <Box>
                  {player.buyer_name}
                  {player.buyer_club_id === buyerClubId && (
                    <Typography variant="caption" color="text.secondary" display="block">
                      your squad
                    </Typography>
                  )}
                </Box>
              ) : (
                '—'
              )}
            </TableCell>
            <TableCell align="right">{player.price ?? '—'}</TableCell>
            <TableCell>
              <RoleChips roles={player.roles} />
            </TableCell>
            <TableCell align="right">
              <Stack direction="row" spacing={1} justifyContent="flex-end">
                {player.state === 'available' && onSell && (
                  <Button size="small" disabled={working} onClick={() => onSell(player)}>
                    Sold
                  </Button>
                )}
                {player.state === 'available' && onUnsold && (
                  <Button size="small" disabled={working} onClick={() => onUnsold(player)}>
                    Unsold
                  </Button>
                )}
                {player.state !== 'available' && onUndo && (
                  <Button size="small" disabled={working} onClick={() => onUndo(player)}>
                    Undo
                  </Button>
                )}
              </Stack>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
    {players.length === 0 && (
      <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>
        Nobody is on this list yet. Search for a player and add him.
      </Typography>
    )}
  </TableContainer>
);

export default AuctionPlayerList;
