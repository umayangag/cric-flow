import { describe, it, expect, beforeEach, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useAuctionProjection } from './useAuctionProjection';
import type { AuctionProjection, AuctionResponse } from '../types';

const mockProject = vi.fn();
const mockSetAssumptions = vi.fn();
const mockSuggestion = vi.fn();

vi.mock('../api', () => ({
  api: {
    projectAuctionCandidate: (...args: unknown[]) => mockProject(...args),
    setAuctionAssumptions: (...args: unknown[]) => mockSetAssumptions(...args),
    auctionOppositionSuggestion: (...args: unknown[]) => mockSuggestion(...args),
  },
}));

/**
 * The projection hook (P3-2).
 *
 * The behaviour worth pinning is what happens when an assumption changes: the projection on
 * screen was computed under the old one, so it is cleared rather than left beside the new
 * one. A range labelled with an eleven it was not computed for is the silent substitution
 * §8.7 forbids, and it is the one thing this hook can get wrong on its own.
 */

const projection = { auction_id: 'auction-1' } as unknown as AuctionProjection;
const record = { auction: { id: 'auction-1' } } as unknown as AuctionResponse;

beforeEach(() => {
  vi.clearAllMocks();
  mockProject.mockResolvedValue(projection);
  mockSetAssumptions.mockResolvedValue(record);
  mockSuggestion.mockResolvedValue({ club_id: 22 });
});

describe('useAuctionProjection', () => {
  it('projects the candidate under the toss and the mix it was given', async () => {
    const { result } = renderHook(() => useAuctionProjection('auction-1', vi.fn()));

    await act(async () => {
      await result.current.project({
        playerId: 2,
        team1BatsFirst: true,
        venueWeights: [{ venue_id: 4, weight: 1 }],
      });
    });

    expect(mockProject).toHaveBeenCalledWith('auction-1', {
      player_id: 2,
      team1_bats_first: true,
      venue_weights: [{ venue_id: 4, weight: 1 }],
    });
    await waitFor(() => expect(result.current.projection).toEqual(projection));
  });

  it('clears the projection when an assumption it was computed under changes', async () => {
    const onRecord = vi.fn();
    const { result } = renderHook(() => useAuctionProjection('auction-1', onRecord));

    await act(async () => {
      await result.current.project({ playerId: 2 });
    });
    await waitFor(() => expect(result.current.projection).toEqual(projection));

    await act(async () => {
      await result.current.saveAssumptions({ likely_xi: [1, 2] });
    });

    expect(onRecord).toHaveBeenCalledWith(record);
    await waitFor(() => expect(result.current.projection).toBeNull());
  });

  it('does nothing at all while no auction is open', async () => {
    const { result } = renderHook(() => useAuctionProjection(null, vi.fn()));

    await act(async () => {
      expect(await result.current.project({ playerId: 2 })).toBeNull();
      expect(await result.current.saveAssumptions({ likely_xi: [] })).toBeNull();
      expect(await result.current.suggestOpposition(22)).toBeNull();
    });

    expect(mockProject).not.toHaveBeenCalled();
    expect(mockSetAssumptions).not.toHaveBeenCalled();
    expect(mockSuggestion).not.toHaveBeenCalled();
  });

  it('asks for a side’s last eleven by club id', async () => {
    const { result } = renderHook(() => useAuctionProjection('auction-1', vi.fn()));

    await act(async () => {
      await result.current.suggestOpposition(22);
    });

    expect(mockSuggestion).toHaveBeenCalledWith('auction-1', 22);
    await waitFor(() => expect(result.current.suggestion).toEqual({ club_id: 22 }));
  });

  it('clears a projection on request, so a refusal never sits beside stale numbers', async () => {
    const { result } = renderHook(() => useAuctionProjection('auction-1', vi.fn()));

    await act(async () => {
      await result.current.project({ playerId: 2 });
    });
    await waitFor(() => expect(result.current.projection).toEqual(projection));

    act(() => result.current.clearProjection());

    expect(result.current.projection).toBeNull();
  });
});
