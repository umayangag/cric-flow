import { describe, it, expect, beforeEach, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { parseMinBowlers, parsePlayerIds, tossRequestFrom, useTeamLab } from './useTeamLab';
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

describe('useTeamLab', () => {
  beforeEach(() => {
    mockGetFormats.mockReset().mockResolvedValue(['T20I']);
    mockGetTeamSidesByFormat.mockReset().mockResolvedValue([indiaMen, indiaWomen]);
    mockGetOpponentSides.mockReset().mockResolvedValue([australiaWomen]);
    mockPredict.mockReset().mockResolvedValue({ team1: [], team2: [] });
    mockOpsStatus.mockReset().mockResolvedValue(null);
    mockSearchVenues.mockReset().mockResolvedValue([]);
  });

  it('cascades formats to sides to opponents, addressing the club by id', async () => {
    const { result } = renderHook(() => useTeamLab());
    await waitFor(() => expect(result.current.availableFormats).toEqual(['T20I']));

    act(() => result.current.setFormat('T20I'));
    await waitFor(() => expect(mockGetTeamSidesByFormat).toHaveBeenCalledWith('T20I'));
    expect(result.current.availableTeam1s).toEqual([indiaMen, indiaWomen]);

    act(() => result.current.setTeam1(indiaWomen));
    await waitFor(() => expect(mockGetOpponentSides).toHaveBeenCalledWith('T20I', 132));
    expect(result.current.availableTeam2s).toEqual([australiaWomen]);
  });

  it('refuses a date outside the offered window', async () => {
    const { result } = renderHook(() => useTeamLab());

    act(() => result.current.setMatchDate('2000-01-01'));
    expect(result.current.dateError).toMatch(/today or in the future/);

    act(() => result.current.setMatchDate('2100-01-01'));
    expect(result.current.dateError).toMatch(/within 14 days/);
  });

  it('sends each side by its club id, and nothing the API retired', async () => {
    const { result } = renderHook(() => useTeamLab());
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
      // The toss starts unknown, which is sent by omission: the simulator then draws
      // both batting orders, which is what it has always done (P1-1).
      team1_bats_first: undefined,
      min_bowlers: undefined,
      require_keeper: true,
      extra_team1: undefined,
      extra_team2: undefined,
    });
  });

  // The toss has three states and each is a different request. "Unknown" is not a missing
  // answer -- it is the marginalised behaviour -- so it is sent by leaving the field out.
  it('sends each toss state, and unknown by omission', async () => {
    const { result } = renderHook(() => useTeamLab());
    act(() => {
      result.current.setFormat('T20I');
      result.current.setTeam1(indiaWomen);
      result.current.setTeam2(australiaWomen);
      result.current.setMatchDate(tomorrow());
    });
    await waitFor(() => expect(result.current.canPredict).toBe(true));

    act(() => result.current.setToss('team1_bats_first'));
    await act(() => result.current.handlePredict());
    expect(mockPredict.mock.calls.at(-1)?.[0].team1_bats_first).toBe(true);

    act(() => result.current.setToss('team2_bats_first'));
    await act(() => result.current.handlePredict());
    expect(mockPredict.mock.calls.at(-1)?.[0].team1_bats_first).toBe(false);

    act(() => result.current.setToss('unknown'));
    await act(() => result.current.handlePredict());
    expect(mockPredict.mock.calls.at(-1)?.[0].team1_bats_first).toBeUndefined();
  });

  it('sends the constraints an eleven is picked under', async () => {
    const { result } = renderHook(() => useTeamLab());
    act(() => {
      result.current.setFormat('T20I');
      result.current.setTeam1(indiaWomen);
      result.current.setTeam2(australiaWomen);
      result.current.setMatchDate(tomorrow());
    });
    await waitFor(() => expect(result.current.canPredict).toBe(true));
    act(() =>
      result.current.setConstraints({
        minBowlers: '5',
        requireKeeper: false,
        extraTeam1: '4021, 5518',
        extraTeam2: '',
      }),
    );
    await waitFor(() => expect(result.current.constraintsError).toBeNull());

    await act(() => result.current.handlePredict());

    expect(mockPredict).toHaveBeenCalledWith(
      expect.objectContaining({
        min_bowlers: 5,
        require_keeper: false,
        extra_team1: [4021, 5518],
        extra_team2: undefined,
      }),
    );
  });

  // A player id nobody can read stops the prediction rather than being dropped from the
  // request: a pool quietly missing the player a user typed is the silence D-12 was.
  it('refuses to predict on a player id it cannot read', async () => {
    const { result } = renderHook(() => useTeamLab());
    act(() => {
      result.current.setFormat('T20I');
      result.current.setTeam1(indiaWomen);
      result.current.setTeam2(australiaWomen);
      result.current.setMatchDate(tomorrow());
    });
    await waitFor(() => expect(result.current.canPredict).toBe(true));

    act(() =>
      result.current.setConstraints({
        minBowlers: '',
        requireKeeper: true,
        extraTeam1: '4021, Smith',
        extraTeam2: '',
      }),
    );

    expect(result.current.constraintsError).toMatch(/not ids: Smith/);
    expect(result.current.canPredict).toBe(false);
    await act(() => result.current.handlePredict());
    expect(mockPredict).not.toHaveBeenCalled();
  });

  // The default pool is the per-format recency window, and asking for it is saying
  // nothing: an omitted `team1_pool` is what the backend reads as the default (D-12).
  it('asks for nothing about the pool by default', async () => {
    const { result } = renderHook(() => useTeamLab());
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
    const { result } = renderHook(() => useTeamLab());
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
    const { result } = renderHook(() => useTeamLab());
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
    const { result } = renderHook(() => useTeamLab());

    await act(() => result.current.handlePredict());

    expect(mockPredict).not.toHaveBeenCalled();
  });

  // The chosen side is kept or dropped by club id. Matching on the name would leave the
  // women's side selected against a list that only offers the men's — a request for one
  // side under the label of the other, which is D-10 with extra steps.
  it('drops a chosen side the new format does not offer', async () => {
    const { result } = renderHook(() => useTeamLab());
    act(() => result.current.setFormat('T20I'));
    await waitFor(() => expect(result.current.availableTeam1s).toHaveLength(2));
    act(() => result.current.setTeam1(indiaWomen));
    await waitFor(() => expect(result.current.team1).toEqual(indiaWomen));

    mockGetTeamSidesByFormat.mockResolvedValue([indiaMen]);
    act(() => result.current.setFormat('TEST'));

    await waitFor(() => expect(result.current.team1).toBeNull());
  });
});

describe('tossRequestFrom', () => {
  it('sends true, false and nothing at all for the three states', () => {
    expect(tossRequestFrom('team1_bats_first')).toBe(true);
    expect(tossRequestFrom('team2_bats_first')).toBe(false);
    expect(tossRequestFrom('unknown')).toBeUndefined();
  });
});

describe('parsePlayerIds', () => {
  it('reads ids separated by commas or spaces', () => {
    expect(parsePlayerIds(' 4021, 5518  77 ')).toEqual({ ids: [4021, 5518, 77], unreadable: [] });
  });

  it('reads nothing out of an empty field', () => {
    expect(parsePlayerIds('   ')).toEqual({ ids: [], unreadable: [] });
  });

  // Keeping the unreadable tokens is the point: an id list that quietly loses a typo would
  // send a pool the user did not ask for, and nothing on the answer would say so.
  it('keeps what it could not read rather than dropping it', () => {
    expect(parsePlayerIds('4021, Smith, -3, 2.5')).toEqual({
      ids: [4021],
      unreadable: ['Smith', '-3', '2.5'],
    });
  });
});

describe('parseMinBowlers', () => {
  it('reads a whole number and leaves the default to config otherwise', () => {
    expect(parseMinBowlers('5')).toBe(5);
    expect(parseMinBowlers('')).toBeUndefined();
    expect(parseMinBowlers('0')).toBeUndefined();
    expect(parseMinBowlers('three')).toBeUndefined();
  });
});
