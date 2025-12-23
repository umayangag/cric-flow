// @vitest-environment jsdom
import React from 'react';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
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
    (api.getMatchSquads as any).mockResolvedValue({
      match_id: 99,
      date: '2019-01-10',
      teams: ['Alpha', 'Beta'] as [string, string],
      squads: [
        { team_name: 'Alpha', actual_win: 1, players: [{ player_name: 'a1', runs_scored: 0, balls_faced: 0, fours_scored: 0, sixes_scored: 0, batting_position: 1, strike_rate: 0, runs_conceded: 0, deliveries: 0, wickets_taken: 0, econ: 0 }] },
        { team_name: 'Beta', actual_win: 0, players: [{ player_name: 'b1', runs_scored: 0, balls_faced: 0, fours_scored: 0, sixes_scored: 0, batting_position: 1, strike_rate: 0, runs_conceded: 0, deliveries: 0, wickets_taken: 0, econ: 0 }] },
      ],
    });
    (api.predictWin as any)
      .mockResolvedValueOnce({ players: [], team_win_probability: 0.8 }) // Alpha
      .mockResolvedValueOnce({ players: [], team_win_probability: 0.2 }); // Beta

    render(<MatchCompareDbTab />);

    fireEvent.change(screen.getByLabelText('match-id-input') as HTMLInputElement, { target: { value: '99' } });
    fireEvent.change(screen.getByLabelText('asof-input') as HTMLInputElement, { target: { value: '2018-12-31' } });
    fireEvent.click(screen.getByLabelText('compare-button'));

    // Predicted/Actual winner should show Alpha and Correct outcome
    await screen.findByText(/Predicted winner/i);
    expect(screen.getByText('Alpha')).toBeInTheDocument();
    expect(screen.getByText(/Actual winner/i)).toBeInTheDocument();
    expect(screen.getByText(/Correct/i)).toBeInTheDocument();
  });

  it('shows friendly error on 404 not found', async () => {
    (api.getMatchSquads as any).mockRejectedValue(new Error('HTTP 404 Not Found: {"code":"NOT_FOUND"}'));

    render(<MatchCompareDbTab />);
    fireEvent.change(screen.getByLabelText('match-id-input') as HTMLInputElement, { target: { value: '1' } });
    fireEvent.click(screen.getByLabelText('compare-button'));

    await screen.findByRole('alert');
    expect(screen.getByText(/Match not found/i)).toBeInTheDocument();
  });

  it('shows friendly error on 422 incomplete squads', async () => {
    (api.getMatchSquads as any).mockRejectedValue(new Error('HTTP 422 Unprocessable Entity: {"code":"INCOMPLETE_SQUADS"}'));

    render(<MatchCompareDbTab />);
    fireEvent.change(screen.getByLabelText('match-id-input') as HTMLInputElement, { target: { value: '2' } });
    fireEvent.click(screen.getByLabelText('compare-button'));

    await screen.findByRole('alert');
    expect(screen.getByText(/Incomplete squads/i)).toBeInTheDocument();
  });

  it('handles probability tie and shows tie as predicted outcome', async () => {
    (api.getMatchSquads as any).mockResolvedValue({
      match_id: 100,
      date: '2019-01-12',
      teams: ['Gamma', 'Delta'] as [string, string],
      squads: [
        { team_name: 'Gamma', actual_win: 0, players: [{ player_name: 'g1', runs_scored: 0, balls_faced: 0, fours_scored: 0, sixes_scored: 0, batting_position: 1, strike_rate: 0, runs_conceded: 0, deliveries: 0, wickets_taken: 0, econ: 0 }] },
        { team_name: 'Delta', actual_win: 1, players: [{ player_name: 'd1', runs_scored: 0, balls_faced: 0, fours_scored: 0, sixes_scored: 0, batting_position: 1, strike_rate: 0, runs_conceded: 0, deliveries: 0, wickets_taken: 0, econ: 0 }] },
      ],
    });
    (api.predictWin as any)
      .mockResolvedValueOnce({ players: [], team_win_probability: 0.5 })
      .mockResolvedValueOnce({ players: [], team_win_probability: 0.5 });

    render(<MatchCompareDbTab />);
    fireEvent.change(screen.getByLabelText('match-id-input') as HTMLInputElement, { target: { value: '100' } });
    fireEvent.click(screen.getByLabelText('compare-button'));

    await screen.findByText(/Predicted winner/i);
    expect(screen.getByText(/tie/i)).toBeInTheDocument();
  });
});
