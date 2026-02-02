// @vitest-environment jsdom
import React from 'react';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import type { Mock } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';
import EvaluateDbTab from '../components/EvaluateDbTab';

// Mock the api module to use the new backtest endpoints
vi.mock('../api', async () => {
  return {
    api: {
      backtestSelect: vi.fn(),
      backtestEvaluate: vi.fn(),
    },
  };
});

const { api } = await import('../api');

describe('EvaluateDbTab (Backtest flow)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('happy path: loads candidates, selects a match, evaluates and renders player MAE', async () => {
    // Arrange mocks
    const backtestSelectMock = api.backtestSelect as unknown as Mock;
    backtestSelectMock.mockResolvedValue({
      filters: { format: 'T20', team1: 'IND', team2: 'AUS' },
      candidates: [
        {
          match_id: 111,
          stable_id: 'x',
          date: '2024-10-30T14:00:00Z',
          venue: 'Wankhede',
          season: '2024',
          format: 'T20',
          team1: 'IND',
          team2: 'AUS',
          winner_team_code: 'IND',
        },
      ],
    });
    const backtestEvaluateMock = api.backtestEvaluate as unknown as Mock;
    backtestEvaluateMock.mockResolvedValue({
      filters: { format: 'T20', team1: 'IND', team2: 'AUS', match_id: 111 },
      match: { match_id: 111, date: '2024-10-30T14:00:00Z' },
      players: [
        {
          player_id: 1,
          predicted: { runs: 25 },
          actual: { runs: 30 },
          errors: { runs_mae: 5 },
        },
        {
          player_id: 2,
          predicted: { runs: 10 },
          actual: { runs: 10 },
          errors: { runs_mae: 0 },
        },
      ],
      metrics: { player_runs_mae: 2.5 },
    });

    render(<EvaluateDbTab />);

    // Inputs exist
    fireEvent.change(screen.getByLabelText(/Format/i), {
      target: { value: 'T20' },
    });
    fireEvent.change(screen.getByLabelText(/Team 1/i), {
      target: { value: 'IND' },
    });
    fireEvent.change(screen.getByLabelText(/Team 2/i), {
      target: { value: 'AUS' },
    });

    // Load candidates
    fireEvent.click(screen.getByRole('button', { name: /Load Played Matches/i }));
    await screen.findByText(/Loaded 1 candidates/i);

    // Select and evaluate
    const radio = screen.getByRole('radio', { name: /Select/i });
    fireEvent.click(radio);
    fireEvent.click(screen.getByRole('button', { name: /Evaluate Selected Match/i }));

    // Expect results
    await screen.findByText(/Evaluation complete/i);
    const results = await screen.findByLabelText('results-section');
    expect(within(results).getByText(/player_runs_mae/i)).toBeInTheDocument();
    // players table should show player ids
    expect(within(results).getByText('1')).toBeInTheDocument();
    expect(within(results).getByText('2')).toBeInTheDocument();
  });

  it('shows error when backtestSelect fails', async () => {
    const backtestSelectMock = api.backtestSelect as unknown as Mock;
    backtestSelectMock.mockRejectedValue(new Error('HTTP 500 Internal Server Error'));

    render(<EvaluateDbTab />);
    fireEvent.click(screen.getByRole('button', { name: /Load Played Matches/i }));
    await screen.findByText(/Error:/i);
  });

  it('renders bowling metrics and match aggregates when present', async () => {
    // Arrange candidates
    const backtestSelectMock = api.backtestSelect as unknown as Mock;
    backtestSelectMock.mockResolvedValue({
      filters: { format: 'T20', team1: 'IND', team2: 'AUS' },
      candidates: [
        {
          match_id: 222,
          stable_id: 'x2',
          date: '2024-11-05T09:00:00Z',
          venue: 'Wankhede',
          season: '2024',
          format: 'T20',
          team1: 'IND',
          team2: 'AUS',
          winner_team_code: 'IND',
        },
      ],
    });

    // Arrange evaluate with wickets/economy and match_aggregates
    const backtestEvaluateMock = api.backtestEvaluate as unknown as Mock;
    backtestEvaluateMock.mockResolvedValue({
      filters: { format: 'T20', team1: 'IND', team2: 'AUS', match_id: 222 },
      match: { match_id: 222, date: '2024-11-05T09:00:00Z' },
      players: [
        {
          player_id: 101,
          predicted: { runs: 28, wickets: 1, economy: 8.0 },
          actual: { runs: 30, wickets: 2, economy: 7.5 },
          errors: { runs_mae: 2, wickets_mae: 1, economy_mae: 0.5 },
        },
        {
          player_id: 102,
          predicted: { runs: 10, wickets: 0, economy: 5.5 },
          actual: { runs: 5, wickets: 0, economy: 6.0 },
          errors: { runs_mae: 5, wickets_mae: 0, economy_mae: 0.5 },
        },
      ],
      match_aggregates: {
        predicted: {
          runs: 160,
          wickets: 6,
          extras: 12,
          winner_team_code: 'IND',
        },
        actual: { runs: 150, wickets: 7, extras: 10, winner_team_code: 'IND' },
        errors: { runs_mae: 10, wickets_mae: 1, extras_mae: 2 },
      },
      metrics: {
        player_runs_mae: 3.667,
        player_wickets_mae: 0.5,
        player_economy_mae: 0.5,
        match_runs_mae: 10,
        match_wickets_mae: 1,
        match_extras_mae: 2,
        winner_accuracy: 1,
      },
    });

    render(<EvaluateDbTab />);

    // Load candidates
    fireEvent.click(screen.getByRole('button', { name: /Load Played Matches/i }));
    await screen.findByText(/Loaded 1 candidates/i);

    // Select and evaluate
    fireEvent.click(screen.getByRole('radio', { name: /Select/i }));
    fireEvent.click(screen.getByRole('button', { name: /Evaluate Selected Match/i }));

    // Assert metrics header shows bowling and match-level metrics
    const results = await screen.findByLabelText('results-section');
    expect(within(results).getByText(/player_wickets_mae/i)).toBeInTheDocument();
    expect(within(results).getByText(/player_economy_mae/i)).toBeInTheDocument();
    expect(within(results).getByText(/match_runs_mae/i)).toBeInTheDocument();
    expect(within(results).getByText(/match_wickets_mae/i)).toBeInTheDocument();
    expect(within(results).getByText(/match_extras_mae/i)).toBeInTheDocument();
    expect(within(results).getByText(/winner_accuracy/i)).toBeInTheDocument();

    // Assert match aggregates block present
    expect(within(results).getByText(/Match aggregates/i)).toBeInTheDocument();
    expect(within(results).getByText(/Predicted/i)).toBeInTheDocument();
    // Multiple elements may contain the word "Actual" (header and table column names)
    const actualLabels = within(results).getAllByText(/Actual\b/i);
    expect(actualLabels.length).toBeGreaterThan(0);
    expect(within(results).getByText(/Errors/i)).toBeInTheDocument();

    // Assert bowling columns appear in table
    const table = within(results).getByRole('table');
    expect(within(table).getByText(/Pred Wkts/i)).toBeInTheDocument();
    expect(within(table).getByText(/Actual Wkts/i)).toBeInTheDocument();
    expect(within(table).getByText(/Wkts Abs Err/i)).toBeInTheDocument();
    expect(within(table).getByText(/Pred Econ/i)).toBeInTheDocument();
    expect(within(table).getByText(/Actual Econ/i)).toBeInTheDocument();
    expect(within(table).getByText(/Econ Abs Err/i)).toBeInTheDocument();
  });

  it('renders fielding metrics (catches, run_outs) and summary metrics when present', async () => {
    // Arrange candidates
    const backtestSelectMock = api.backtestSelect as unknown as Mock;
    backtestSelectMock.mockResolvedValue({
      filters: { format: 'T20', team1: 'IND', team2: 'AUS' },
      candidates: [
        {
          match_id: 333,
          stable_id: 'x3',
          date: '2024-11-06T09:00:00Z',
          venue: 'Wankhede',
          season: '2024',
          format: 'T20',
          team1: 'IND',
          team2: 'AUS',
          winner_team_code: 'IND',
        },
      ],
    });

    // Arrange evaluate with fielding keys present
    const backtestEvaluateMock = api.backtestEvaluate as unknown as Mock;
    backtestEvaluateMock.mockResolvedValue({
      filters: { format: 'T20', team1: 'IND', team2: 'AUS', match_id: 333 },
      match: { match_id: 333, date: '2024-11-06T09:00:00Z' },
      players: [
        {
          player_id: 201,
          predicted: { runs: 12, catches: 1, run_outs: 2 },
          actual: { runs: 10, catches: 2, run_outs: 1 },
          errors: { runs_mae: 2, catches_mae: 1, run_outs_mae: 1 },
        },
        {
          player_id: 202,
          predicted: { runs: 4, catches: 0, run_outs: 1 },
          actual: { runs: 5, catches: 0, run_outs: 0 },
          errors: { runs_mae: 1, catches_mae: 0, run_outs_mae: 1 },
        },
      ],
      metrics: {
        player_runs_mae: 1.5,
        player_catches_mae: 0.5,
        player_run_outs_mae: 1.0,
      },
    });

    render(<EvaluateDbTab />);

    // Load candidates
    fireEvent.click(screen.getByRole('button', { name: /Load Played Matches/i }));
    await screen.findByText(/Loaded 1 candidates/i);

    // Select and evaluate
    fireEvent.click(screen.getByRole('radio', { name: /Select/i }));
    fireEvent.click(screen.getByRole('button', { name: /Evaluate Selected Match/i }));

    // Assert metrics header shows fielding metrics
    const results = await screen.findByLabelText('results-section');
    expect(within(results).getByText(/player_catches_mae/i)).toBeInTheDocument();
    expect(within(results).getByText(/player_run_outs_mae/i)).toBeInTheDocument();

    // Assert fielding columns appear in table
    const table = within(results).getByRole('table');
    expect(within(table).getByText(/Pred Catches/i)).toBeInTheDocument();
    expect(within(table).getByText(/Actual Catches/i)).toBeInTheDocument();
    expect(within(table).getByText(/Catches Abs Err/i)).toBeInTheDocument();
    expect(within(table).getByText(/Pred Run Outs/i)).toBeInTheDocument();
    expect(within(table).getByText(/Actual Run Outs/i)).toBeInTheDocument();
    expect(within(table).getByText(/Run Outs Abs Err/i)).toBeInTheDocument();
  });

  it('disable evaluate until a candidate match is selected', async () => {
    const backtestSelectMock = api.backtestSelect as unknown as Mock;
    backtestSelectMock.mockResolvedValue({
      filters: {},
      candidates: [],
    });
    render(<EvaluateDbTab />);
    fireEvent.click(screen.getByRole('button', { name: /Load Played Matches/i }));
    await screen.findByText(/No candidates loaded yet/i);
    const btn = screen.getByRole('button', {
      name: /Evaluate Selected Match/i,
    }) as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
  });
});
