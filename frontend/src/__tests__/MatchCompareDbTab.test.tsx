// @vitest-environment jsdom
import React from 'react';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import type { Mock } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';
import MatchCompareDbTab from '../components/MatchCompareDbTab';

vi.mock('../api', async () => {
  return {
    api: {
      getMatchSquads: vi.fn(),
      predictWin: vi.fn(),
    },
  };
});

const { api } = await import('../api');

describe('MatchCompareDbTab', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('happy path: compares predictions and shows correct/incorrect outcome', async () => {
    const getMatchSquadsMock = api.getMatchSquads as unknown as Mock;
    getMatchSquadsMock.mockResolvedValue({
      match_id: 99,
      date: '2019-01-10',
      teams: ['Alpha', 'Beta'] as [string, string],
      squads: [
        {
          team_name: 'Alpha',
          actual_win: 1,
          players: [
            {
              player_name: 'a1',
              runs_scored: 0,
              balls_faced: 0,
              fours_scored: 0,
              sixes_scored: 0,
              batting_position: 1,
              strike_rate: 0,
              runs_conceded: 0,
              deliveries: 0,
              wickets_taken: 0,
              econ: 0,
            },
          ],
        },
        {
          team_name: 'Beta',
          actual_win: 0,
          players: [
            {
              player_name: 'b1',
              runs_scored: 0,
              balls_faced: 0,
              fours_scored: 0,
              sixes_scored: 0,
              batting_position: 1,
              strike_rate: 0,
              runs_conceded: 0,
              deliveries: 0,
              wickets_taken: 0,
              econ: 0,
            },
          ],
        },
      ],
    });
    const predictWinMock = api.predictWin as unknown as Mock;
    predictWinMock
      .mockResolvedValueOnce({ players: [], team_win_probability: 0.8 }) // Alpha
      .mockResolvedValueOnce({ players: [], team_win_probability: 0.2 }); // Beta

    render(<MatchCompareDbTab />);

    fireEvent.change(screen.getAllByLabelText('match-id-input')[0] as HTMLInputElement, {
      target: { value: '99' },
    });
    fireEvent.change(screen.getAllByLabelText('asof-input')[0] as HTMLInputElement, {
      target: { value: '2018-12-31' },
    });
    fireEvent.click(screen.getAllByLabelText('compare-button')[0]);

    // Predicted/Actual winner should show Alpha and Correct outcome
    const predictedRow = await screen.findByText(/Predicted winner/i);
    // Scope strictly to the row element that contains the "Predicted winner" label
    expect(within(predictedRow as HTMLElement).getByText('Alpha')).toBeInTheDocument();
    const actualRow = screen.getByText(/Actual winner/i);
    expect(within(actualRow as HTMLElement).getByText('Alpha')).toBeInTheDocument();
    expect(screen.getByText(/Correct/i)).toBeInTheDocument();
  });

  it('shows friendly error on 404 not found', async () => {
    const getMatchSquadsMock = api.getMatchSquads as unknown as Mock;
    getMatchSquadsMock.mockRejectedValue(new Error('HTTP 404 Not Found: {"code":"NOT_FOUND"}'));

    render(<MatchCompareDbTab />);
    fireEvent.change(screen.getAllByLabelText('match-id-input')[0] as HTMLInputElement, {
      target: { value: '1' },
    });
    fireEvent.click(screen.getAllByLabelText('compare-button')[0]);

    await screen.findByRole('alert');
    expect(screen.getByText(/Match not found/i)).toBeInTheDocument();
  });

  it('shows friendly error on 422 incomplete squads', async () => {
    const getMatchSquadsMock = api.getMatchSquads as unknown as Mock;
    getMatchSquadsMock.mockRejectedValue(
      new Error('HTTP 422 Unprocessable Entity: {"code":"INCOMPLETE_SQUADS"}'),
    );

    render(<MatchCompareDbTab />);
    fireEvent.change(screen.getAllByLabelText('match-id-input')[0] as HTMLInputElement, {
      target: { value: '2' },
    });
    fireEvent.click(screen.getAllByLabelText('compare-button')[0]);

    await screen.findByRole('alert');
    expect(screen.getByText(/Incomplete squads/i)).toBeInTheDocument();
  });

  it('handles probability tie and shows tie as predicted outcome', async () => {
    const getMatchSquadsMock = api.getMatchSquads as unknown as Mock;
    getMatchSquadsMock.mockResolvedValue({
      match_id: 100,
      date: '2019-01-12',
      teams: ['Gamma', 'Delta'] as [string, string],
      squads: [
        {
          team_name: 'Gamma',
          actual_win: 0,
          players: [
            {
              player_name: 'g1',
              runs_scored: 0,
              balls_faced: 0,
              fours_scored: 0,
              sixes_scored: 0,
              batting_position: 1,
              strike_rate: 0,
              runs_conceded: 0,
              deliveries: 0,
              wickets_taken: 0,
              econ: 0,
            },
          ],
        },
        {
          team_name: 'Delta',
          actual_win: 1,
          players: [
            {
              player_name: 'd1',
              runs_scored: 0,
              balls_faced: 0,
              fours_scored: 0,
              sixes_scored: 0,
              batting_position: 1,
              strike_rate: 0,
              runs_conceded: 0,
              deliveries: 0,
              wickets_taken: 0,
              econ: 0,
            },
          ],
        },
      ],
    });
    const predictWinMock = api.predictWin as unknown as Mock;
    predictWinMock
      .mockResolvedValueOnce({ players: [], team_win_probability: 0.5 })
      .mockResolvedValueOnce({ players: [], team_win_probability: 0.5 });

    render(<MatchCompareDbTab />);
    fireEvent.change(screen.getAllByLabelText('match-id-input')[0] as HTMLInputElement, {
      target: { value: '100' },
    });
    fireEvent.click(screen.getAllByLabelText('compare-button')[0]);

    await screen.findByText(/Predicted winner/i);
    expect(screen.getByText(/tie/i)).toBeInTheDocument();
  });
});
