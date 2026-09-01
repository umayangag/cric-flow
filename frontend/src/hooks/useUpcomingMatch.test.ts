import { describe, it, expect, beforeEach, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useUpcomingMatch } from './useUpcomingMatch';

const mockGetFormats = vi.fn();
const mockGetTeamsByFormat = vi.fn();
const mockGetOpponents = vi.fn();
const mockPredict = vi.fn();
const mockOpsStatus = vi.fn();
const mockSearchVenues = vi.fn();
vi.mock('../api', () => ({
  api: {
    getFormats: (...args: unknown[]) => mockGetFormats(...args),
    getTeamsByFormat: (...args: unknown[]) => mockGetTeamsByFormat(...args),
    getOpponents: (...args: unknown[]) => mockGetOpponents(...args),
    predictTeamSelection: (...args: unknown[]) => mockPredict(...args),
    opsStatus: (...args: unknown[]) => mockOpsStatus(...args),
    searchVenues: (...args: unknown[]) => mockSearchVenues(...args),
  },
}));

function tomorrow(): string {
  return new Date(Date.now() + 24 * 3600 * 1000).toISOString().slice(0, 10);
}

describe('useUpcomingMatch', () => {
  beforeEach(() => {
    mockGetFormats.mockReset().mockResolvedValue(['T20']);
    mockGetTeamsByFormat.mockReset().mockResolvedValue(['IND', 'AUS']);
    mockGetOpponents.mockReset().mockResolvedValue(['AUS']);
    mockPredict.mockReset().mockResolvedValue({ team1: [], team2: [] });
    mockOpsStatus.mockReset().mockResolvedValue(null);
    mockSearchVenues.mockReset().mockResolvedValue([]);
  });

  it('cascades formats to teams to opponents', async () => {
    const { result } = renderHook(() => useUpcomingMatch());
    await waitFor(() => expect(result.current.availableFormats).toEqual(['T20']));

    act(() => result.current.setFormat('T20'));
    await waitFor(() => expect(mockGetTeamsByFormat).toHaveBeenCalledWith('T20'));

    act(() => result.current.setTeam1('IND'));
    await waitFor(() => expect(mockGetOpponents).toHaveBeenCalledWith('T20', 'IND'));
    expect(result.current.availableTeam2s).toEqual(['AUS']);
  });

  it('refuses a date outside the offered window', async () => {
    const { result } = renderHook(() => useUpcomingMatch());

    act(() => result.current.setMatchDate('2000-01-01'));
    expect(result.current.dateError).toMatch(/today or in the future/);

    act(() => result.current.setMatchDate('2100-01-01'));
    expect(result.current.dateError).toMatch(/within 14 days/);
  });

  it('sends only the fixture, and nothing the API retired', async () => {
    const { result } = renderHook(() => useUpcomingMatch());
    act(() => {
      result.current.setFormat('T20');
      result.current.setTeam1('IND');
      result.current.setTeam2('AUS');
      result.current.setMatchDate(tomorrow());
    });
    await waitFor(() => expect(result.current.canPredict).toBe(true));

    await act(() => result.current.handlePredict());

    expect(mockPredict).toHaveBeenCalledWith({
      format: 'T20',
      team1: 'IND',
      team2: 'AUS',
      venue: undefined,
      match_date: tomorrow(),
    });
  });

  it('does not predict while the form is incomplete', async () => {
    const { result } = renderHook(() => useUpcomingMatch());

    await act(() => result.current.handlePredict());

    expect(mockPredict).not.toHaveBeenCalled();
  });
});
