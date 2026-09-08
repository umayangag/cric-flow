import { useCallback, useEffect, useState } from 'react';
import { api } from '../api';
import { useAsync } from './useAsync';
import type { AuctionPlayerState, AuctionResponse, AuctionSummary } from '../types';

/**
 * The auction the operator is running (P3-1).
 *
 * Every write returns the auction as it now stands, so this hook never merges a response
 * into a local copy: the answer *is* the record. A client that patched its own would be
 * one mistyped entry from showing a squad the backend does not hold, and the point of the
 * record is that what is on screen is what was entered.
 *
 * The state is deliberately thin — which auction is open, and its last answer. There is no
 * derived count here: the squad, the open slots and the pool's distribution by role all
 * arrive on the answer, computed from the record and the served rating vectors, and
 * recomputing any of them in the browser would be a second definition of a number the
 * surface is supposed to be reporting.
 */
export type AuctionOutcomeEntry = {
  playerId: number;
  state: AuctionPlayerState;
  buyerName?: string;
  buyerClubId?: number;
  price?: number;
};

export function useAuction() {
  const index = useAsync(api.auctions, {
    errorMessage: 'Failed to list the auctions',
    runOnMount: [],
  });
  const record = useAsync(api.auction, { errorMessage: 'Failed to read the auction' });
  const create = useAsync(api.createAuction, { errorMessage: 'Failed to create the auction' });
  const addPlayers = useAsync(api.addAuctionPlayers, {
    errorMessage: 'Failed to list the players',
  });
  const outcome = useAsync(api.recordAuctionOutcome, {
    errorMessage: 'Failed to record the outcome',
  });

  const [openAuctionId, setOpenAuctionId] = useState<string | null>(null);

  const { run: loadRecord, setData: setRecord } = record;
  const { run: loadIndex } = index;

  // Opening an auction re-reads it rather than reusing whatever the last write returned.
  // A reload of the page is the state the gate is about: what comes back is the record.
  useEffect(() => {
    if (!openAuctionId) return;
    void loadRecord(openAuctionId);
  }, [openAuctionId, loadRecord]);

  const { run: runCreate } = create;
  const createAuction = useCallback(
    async (body: Parameters<typeof api.createAuction>[0]) => {
      const created = await runCreate(body);
      if (!created) return null;
      setRecord(created);
      setOpenAuctionId(created.auction.id);
      void loadIndex();
      return created;
    },
    [runCreate, setRecord, loadIndex],
  );

  const { run: runAddPlayers } = addPlayers;
  const listPlayers = useCallback(
    async (playerIds: number[]) => {
      if (!openAuctionId) return null;
      const listed = await runAddPlayers(openAuctionId, playerIds);
      if (listed) setRecord(listed);
      return listed;
    },
    [openAuctionId, runAddPlayers, setRecord],
  );

  const { run: runOutcome } = outcome;
  const recordOutcome = useCallback(
    async (entry: AuctionOutcomeEntry) => {
      if (!openAuctionId) return null;
      const updated = await runOutcome(openAuctionId, {
        player_id: entry.playerId,
        state: entry.state,
        buyer_name: entry.buyerName,
        buyer_club_id: entry.buyerClubId,
        price: entry.price,
      });
      if (updated) setRecord(updated);
      return updated;
    },
    [openAuctionId, runOutcome, setRecord],
  );

  return {
    auctions: (index.data?.auctions ?? []) as AuctionSummary[],
    auctionsLoading: index.loading,
    openAuctionId,
    openAuction: setOpenAuctionId,
    record: record.data as AuctionResponse | null,
    loading: record.loading,
    working: create.loading || addPlayers.loading || outcome.loading,
    error: record.error ?? create.error ?? addPlayers.error ?? outcome.error ?? index.error,
    createAuction,
    listPlayers,
    recordOutcome,
  };
}
