import React from 'react';
import { Alert, Box, Button, Stack, Typography } from '@mui/material';
import ErrorNotice from './common/ErrorNotice';
import PredictionReadiness from './PredictionReadiness';
import AuctionSetup from './AuctionSetup';
import AuctionRoleDistribution from './AuctionRoleDistribution';
import AuctionPlayerList from './AuctionPlayerList';
import AuctionPlayerSearchDialog from './AuctionPlayerSearchDialog';
import AuctionSaleDialog from './AuctionSaleDialog';
import AuctionAssumptions from './AuctionAssumptions';
import AuctionProjectionPanel from './AuctionProjectionPanel';
import { useAuction } from '../hooks/useAuction';
import { useAuctionProjection } from '../hooks/useAuctionProjection';
import { useAsync } from '../hooks/useAsync';
import { api } from '../api';
import type { AuctionListedPlayer } from '../types';

/**
 * The Auction tab: the record of an auction as the operator enters it, and the remaining
 * pool's distribution by role (P3-1).
 *
 * **The rule this surface is built on.** This module is valuation and projection and never
 * XI-picking. The system's own record is that in T20 optimised selection is
 * indistinguishable from rating order, and the IPL is domestic T20 — so no request from
 * this tab reaches the optimiser, and no probability of winning, no value of a swap and no
 * "best XI" appears anywhere on it. What it shows is the record, and the two role
 * predicates read off the served rating vectors. `opsContract.test.ts` asserts that
 * against these sources; go-app asserts it through its client.
 *
 * Every number on screen is one the backend computed. The squad, the open slots and the
 * pool's distribution all arrive on the answer to each write; nothing is derived here,
 * because a count computed in the browser would be a second definition of a number this
 * surface is meant to be reporting.
 */

/** The record's sentence, kept where the numbers are and not in a footnote. */
export const NOT_XI_PICKING_SENTENCE =
  'In T20 the system has not shown it can choose an eleven better than rating order, and ' +
  'this module does not try to — it projects and values. There is no win probability, no ' +
  'marginal value and no "best XI" on this surface.';

const AuctionTab: React.FC = () => {
  const auction = useAuction();
  const projection = useAuctionProjection(auction.openAuctionId, auction.setRecord);
  const opsStatus = useAsync(api.opsStatus, { runOnMount: [] });

  const [searching, setSearching] = React.useState(false);
  const [selling, setSelling] = React.useState<AuctionListedPlayer | null>(null);

  const record = auction.record;

  return (
    <Box>
      <Typography variant="h6" sx={{ mb: 2 }}>
        Auction
      </Typography>

      <Alert severity="info" sx={{ mb: 2 }} data-testid="auction-not-xi-picking">
        {NOT_XI_PICKING_SENTENCE}
      </Alert>

      <PredictionReadiness status={opsStatus.data} />

      <AuctionSetup
        auctions={auction.auctions}
        openAuctionId={auction.openAuctionId}
        onOpen={auction.openAuction}
        onCreate={(body) => void auction.createAuction(body)}
        working={auction.working}
      />

      <ErrorNotice error={auction.error} title="The auction record failed" />

      {!record && !auction.loading && (
        <Typography variant="body2" color="text.secondary">
          No auction is open. Create one above, or pick one off the record.
        </Typography>
      )}

      {record && (
        <>
          <Stack
            direction="row"
            spacing={2}
            alignItems="center"
            justifyContent="space-between"
            sx={{ mb: 1 }}
            flexWrap="wrap"
            useFlexGap
          >
            <Typography variant="subtitle1" data-testid="auction-name">
              {record.auction.name} — {record.auction.format}, buying for{' '}
              {record.auction.buyer.name}
            </Typography>
            <Button
              variant="outlined"
              size="small"
              disabled={auction.working}
              onClick={() => setSearching(true)}
            >
              Add players
            </Button>
          </Stack>

          <AuctionRoleDistribution
            slots={record.slots}
            distribution={record.distribution}
            roles={record.roles}
          />

          <AuctionPlayerList
            players={record.auction.players}
            buyerClubId={record.auction.buyer.club_id}
            working={auction.working}
            onSell={setSelling}
            onUnsold={(player) =>
              void auction.recordOutcome({ playerId: player.player_id, state: 'unsold' })
            }
            onUndo={(player) =>
              void auction.recordOutcome({ playerId: player.player_id, state: 'available' })
            }
          />

          <AuctionAssumptions
            record={record.auction}
            listed={record.auction.players}
            suggestion={projection.suggestion}
            suggesting={projection.suggesting}
            suggestionError={projection.suggestionError}
            onSuggest={(clubId) => void projection.suggestOpposition(clubId)}
            onSave={(body) => void projection.saveAssumptions(body)}
            saving={projection.savingAssumptions}
            saveError={projection.assumptionsError}
          />

          <AuctionProjectionPanel
            listed={record.auction.players}
            projection={projection.projection}
            projecting={projection.projecting}
            error={projection.projectionError}
            onProject={(request) => void projection.project(request)}
          />

          <AuctionPlayerSearchDialog
            open={searching}
            onClose={() => setSearching(false)}
            format={record.auction.format}
            listedIds={record.auction.players.map((player) => player.player_id)}
            onAdd={(playerIds) => void auction.listPlayers(playerIds)}
          />

          <AuctionSaleDialog
            open={selling !== null}
            onClose={() => setSelling(null)}
            player={selling}
            buyer={record.auction.buyer}
            onConfirm={(sale) => {
              if (!selling) return;
              void auction.recordOutcome({
                playerId: selling.player_id,
                state: 'sold',
                buyerName: sale.buyerName,
                buyerClubId: sale.buyerClubId,
                price: sale.price,
              });
            }}
          />
        </>
      )}
    </Box>
  );
};

export default AuctionTab;
