import { describe, it, expect, beforeEach, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useUpcomingMatch } from './useUpcomingMatch';
import type { TeamSideOption } from '../types';

const mockGetFormats = vi.fn();
const mockGetTeamSidesByFormat = vi.fn();
const mockGetOpponentSides = vi.fn();
const mockPredict = vi.fn();
const mockOpsStatus = vi.fn();
const mockSearchVenues = vi.fn();
vi.mock('../api', () => ({
  api: {
    getFormats: (...args: unknown[]) => mockGetFormats(...args),
    getTeamSidesByFormat: (...args: unknown[]) => mockGetTeamSidesByFormat(...args),
    getOpponentSides: (...args: unknown[]) => mockGetOpponentSides(...args),
    predictTeamSelection: (...args: unknown[]) => mockPredict(...args),
    opsStatus: (...args: unknown[]) => mockOpsStatus(...args),
    searchVenues: (...args: unknown[]) => mockSearchVenues(...args),
  },
}));

// The two sides one name stands for, and the opponent of one of them.
const indiaMen: TeamSideOption = {
  club_id: 43,
  name: 'India',
  gender: 'male',
  display_name: 'India (men)',
};
const indiaWomen: TeamSideOption = {
  club_id: 132,
  name: 'India',
  gender: 'female',
  display_name: 'India (women)',
};
const australiaWomen: TeamSideOption = {
  club_id: 12,
  name: 'Australia',
  gender: 'female',
  display_name: 'Australia (women)',
};

function tomorrow(): string {
  return new Date(Date.now() + 24 * 3600 * 1000).toISOString().slice(0, 10);
}

describe('useUpcomingMatch', () => {
  beforeEach(() => {
    mockGetFormats.mockReset().mockResolvedValue(['T20I']);
    mockGetTeamSidesByFormat.mockReset().mockResolvedValue([indiaMen, indiaWomen]);
    mockGetOpponentSides.mockReset().mockResolvedValue([australiaWomen]);
    mockPredict.mockReset().mockResolvedValue({ team1: [], team2: [] });
    mockOpsStatus.mockReset().mockResolvedValue(null);
    mockSearchVenues.mockReset().mockResolvedValue([]);
  });

  it('cascades formats to sides to opponents, addressing the club by id', async () => {
    const { result } = renderHook(() => useUpcomingMatch());
    await waitFor(() => expect(result.current.availableFormats).toEqual(['T20I']));

    act(() => result.current.setFormat('T20I'));
    await waitFor(() => expect(mockGetTeamSidesByFormat).toHaveBeenCalledWith('T20I'));
    expect(result.current.availableTeam1s).toEqual([indiaMen, indiaWomen]);

    act(() => result.current.setTeam1(indiaWomen));
    await waitFor(() => expect(mockGetOpponentSides).toHaveBeenCalledWith('T20I', 132));
    expect(result.current.availableTeam2s).toEqual([australiaWomen]);
  });

  it('refuses a date outside the offered window', async () => {
    const { result } = renderHook(() => useUpcomingMatch());

    act(() => result.current.setMatchDate('2000-01-01'));
    expect(result.current.dateError).toMatch(/today or in the future/);

    act(() => result.current.setMatchDate('2100-01-01'));
    expect(result.current.dateError).toMatch(/within 14 days/);
  });

  it('sends each side by its club id, and nothing the API retired', async () => {
    const { result } = renderHook(() => useUpcomingMatch());
    act(() => {
      result.current.setFormat('T20I');
      result.current.setTeam1(indiaWomen);
      result.current.setTeam2(australiaWomen);
      result.current.setMatchDate(tomorrow());
    });
    await waitFor(() => expect(result.current.canPredict).toBe(true));

    await act(() => result.current.handlePredict());

    expect(mockPredict).toHaveBeenCalledWith({
      format: 'T20I',
      team1_id: 132,
      team2_id: 12,
      venue: undefined,
      match_date: tomorrow(),
      team1_pool: undefined,
      team2_pool: undefined,
    });
  });

  // The default pool is the per-format recency window, and asking for it is saying
  // nothing: an omitted `team1_pool` is what the backend reads as the default (D-12).
  it('asks for nothing about the pool by default', async () => {
    const { result } = renderHook(() => useUpcomingMatch());
    act(() => {
      result.current.setFormat('T20I');
      result.current.setTeam1(indiaWomen);
      result.current.setTeam2(australiaWomen);
      result.current.setMatchDate(tomorrow());
    });
    await waitFor(() => expect(result.current.canPredict).toBe(true));

    await act(() => result.current.handlePredict());

    expect(mockPredict.mock.calls[0][0].team1_pool).toBeUndefined();
    expect(mockPredict.mock.calls[0][0].team2_pool).toBeUndefined();
  });

  // Widening is a question about the answer on screen, so it predicts again rather than
  // leaving a number the new pool did not produce beside a line that says it did.
  it('widens one side to the all-time pool and predicts again', async () => {
    const { result } = renderHook(() => useUpcomingMatch());
    act(() => {
      result.current.setFormat('T20I');
      result.current.setTeam1(indiaWomen);
      result.current.setTeam2(australiaWomen);
      result.current.setMatchDate(tomorrow());
    });
    await waitFor(() => expect(result.current.canPredict).toBe(true));

    await act(() => result.current.widenPool(1));

    expect(mockPredict).toHaveBeenCalledWith(
      expect.objectContaining({ team1_pool: { all_time: true }, team2_pool: undefined }),
    );
    expect(result.current.team1Pool).toEqual({ allTime: true, players: null });
  });

  // A manual pick is the pool: the ids the user ticked, and neither the window nor the
  // ledger applied to them.
  it('sends a hand-picked pool as the pool', async () => {
    const { result } = renderHook(() => useUpcomingMatch());
    act(() => {
      result.current.setFormat('T20I');
      result.current.setTeam1(indiaWomen);
      result.current.setTeam2(australiaWomen);
      result.current.setMatchDate(tomorrow());
    });
    await waitFor(() => expect(result.current.canPredict).toBe(true));
    act(() => result.current.setTeam2Pool({ allTime: true, players: [4, 9] }));

    await act(() => result.current.handlePredict());

    expect(mockPredict).toHaveBeenCalledWith(
      expect.objectContaining({ team2_pool: { players: [4, 9] } }),
    );
  });

  it('does not predict while the form is incomplete', async () => {
    const { result } = renderHook(() => useUpcomingMatch());

    await act(() => result.current.handlePredict());

    expect(mockPredict).not.toHaveBeenCalled();
  });

  // The chosen side is kept or dropped by club id. Matching on the name would leave the
  // women's side selected against a list that only offers the men's — a request for one
  // side under the label of the other, which is D-10 with extra steps.
  it('drops a chosen side the new format does not offer', async () => {
    const { result } = renderHook(() => useUpcomingMatch());
    act(() => result.current.setFormat('T20I'));
    await waitFor(() => expect(result.current.availableTeam1s).toHaveLength(2));
    act(() => result.current.setTeam1(indiaWomen));
    await waitFor(() => expect(result.current.team1).toEqual(indiaWomen));

    mockGetTeamSidesByFormat.mockResolvedValue([indiaMen]);
    act(() => result.current.setFormat('TEST'));

    await waitFor(() => expect(result.current.team1).toBeNull());
  });
});
