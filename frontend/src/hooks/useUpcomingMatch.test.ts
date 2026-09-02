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
    });
  });

  it('does not predict while the form is incomplete', async () => {
    const { result } = renderHook(() => useUpcomingMatch());

    await act(() => result.current.handlePredict());

    expect(mockPredict).not.toHaveBeenCalled();
  });

  // The chosen side is kept or dropped by club id. Matching on the name would leave the
  // women's side selected against a list that only offers the men's — a request for one
  // side under the label of the other, which is D-11 with extra steps.
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
