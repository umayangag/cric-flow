// @vitest-environment jsdom
import React from 'react';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import type { Mock } from 'vitest';
import { render, screen, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import EvaluateDbTab from '../components/EvaluateDbTab';

// Mock the api module to use the new backtest endpoints
vi.mock('../api', async () => {
  return {
    api: {
      backtestSelect: vi.fn(),
      backtestEvaluate: vi.fn(),
      backtestEvaluateStream: vi.fn(),
      evaluateStart: vi.fn(),
      getEvaluateStatus: vi.fn(),
      getMatchScorecard: vi.fn(),
      getFormats: vi.fn(),
      getTeamsByFormat: vi.fn(),
      getOpponents: vi.fn(),
    },
  };
});

const { api } = await import('../api');

async function selectFilters(
  user: ReturnType<typeof userEvent.setup>,
  team1 = 'IND',
  team2 = 'AUS',
) {
  await waitFor(() => expect(api.getFormats).toHaveBeenCalled());
  await user.click(screen.getByRole('combobox', { name: /Format/i }));
  await user.click(await screen.findByRole('option', { name: 'T20' }));

  await waitFor(() => expect(api.getTeamsByFormat).toHaveBeenCalledWith('T20'));
  const team1Input = screen.getByRole('combobox', { name: /Team 1/i });
  await user.click(team1Input);
  await user.keyboard(team1);
  await user.click(await screen.findByText(team1));

  await waitFor(() => expect(api.getOpponents).toHaveBeenCalledWith('T20', team1));
  const team2Input = screen.getByRole('combobox', { name: /Team 2/i });
  await user.click(team2Input);
  await user.keyboard(team2);
  await user.click(await screen.findByText(team2));
}

describe('EvaluateDbTab (Backtest flow)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    try {
      localStorage.removeItem('cric_info_eval_job');
    } catch {
      /* ignore */
    }
    (api.getFormats as unknown as Mock).mockResolvedValue(['T20', 'ODI']);
    (api.getTeamsByFormat as unknown as Mock).mockResolvedValue(['IND', 'AUS', 'ENG']);
    (api.getOpponents as unknown as Mock).mockResolvedValue(['AUS', 'ENG']);
    (api.getMatchScorecard as unknown as Mock).mockResolvedValue(null);
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
          match_date: '2024-10-30T14:00:00Z',
          venue: 'Wankhede',
          season: '2024',
          format: 'T20',
          team1: 'IND',
          team2: 'AUS',
          winner_team_code: 'IND',
        },
      ],
    });
    const evaluateStartMock = api.evaluateStart as unknown as Mock;
    evaluateStartMock.mockResolvedValue({ job_id: 'test-job-111' });
    const getEvaluateStatusMock = api.getEvaluateStatus as unknown as Mock;
    const resultPayload = {
      filters: { format: 'T20', team1: 'IND', team2: 'AUS', match_id: 111 },
      match: { match_id: 111, match_date: '2024-10-30T14:00:00Z' },
      players: [
        { player_id: 1, predicted: { runs: 25 }, actual: { runs: 30 }, errors: { runs_mae: 5 } },
        { player_id: 2, predicted: { runs: 10 }, actual: { runs: 10 }, errors: { runs_mae: 0 } },
      ],
      metrics: { player_runs_mae: 2.5 },
    };
    getEvaluateStatusMock.mockResolvedValue({
      job_id: 'test-job-111',
      match_id: 111,
      format: 'T20',
      team1: 'IND',
      team2: 'AUS',
      status: 'done',
      result: resultPayload,
    });

    const user = userEvent.setup();
    render(<EvaluateDbTab />);
    await selectFilters(user);

    // Load candidates
    await user.click(screen.getByRole('button', { name: /Load Matches/i }));
    await screen.findByText(/Loaded 1 candidates/i, { exact: false });

    // Select and evaluate
    const radio = screen.getByRole('radio', { name: /Select/i });
    await user.click(radio);
    await user.click(screen.getByRole('button', { name: /Evaluate Selected Match/i }));

    // Wait for polling to run and job to complete (status returns 'done')
    await waitFor(() => expect(getEvaluateStatusMock).toHaveBeenCalled());
    const results = await screen.findByLabelText('results-section', {}, { timeout: 3000 });
    // Status message appears in more than one Alert; ensure at least one shows completion
    await waitFor(() =>
      expect(screen.getAllByText(/Evaluation complete/i).length).toBeGreaterThanOrEqual(1),
    );
    expect(within(results).getByText(/player_runs_mae/i)).toBeInTheDocument();
    // players table should show player ids
    expect(within(results).getByText('1')).toBeInTheDocument();
    expect(within(results).getByText('2')).toBeInTheDocument();
  }, 15000);

  it('shows error when backtestSelect fails', async () => {
    const user = userEvent.setup();
    const backtestSelectMock = api.backtestSelect as unknown as Mock;
    backtestSelectMock.mockRejectedValue(new Error('HTTP 500 Internal Server Error'));

    render(<EvaluateDbTab />);
    await selectFilters(user);
    await user.click(screen.getByRole('button', { name: /Load Matches/i }));
    await screen.findByText(/HTTP 500/i);
  }, 10000);

  it('renders bowling metrics and match aggregates when present', async () => {
    // Arrange candidates
    const backtestSelectMock = api.backtestSelect as unknown as Mock;
    backtestSelectMock.mockResolvedValue({
      filters: { format: 'T20', team1: 'IND', team2: 'AUS' },
      candidates: [
        {
          match_id: 222,
          stable_id: 'x2',
          match_date: '2024-11-05T09:00:00Z',
          venue: 'Wankhede',
          season: '2024',
          format: 'T20',
          team1: 'IND',
          team2: 'AUS',
          winner_team_code: 'IND',
        },
      ],
    });

    const evaluatePayload = {
      filters: { format: 'T20', team1: 'IND', team2: 'AUS', match_id: 222 },
      match: { match_id: 222, match_date: '2024-11-05T09:00:00Z' },
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
    };
    (api.evaluateStart as unknown as Mock).mockResolvedValue({ job_id: 'test-job-222' });
    (api.getEvaluateStatus as unknown as Mock).mockResolvedValue({
      job_id: 'test-job-222',
      match_id: '222',
      format: 'T20',
      team1: 'IND',
      team2: 'AUS',
      status: 'done',
      result: evaluatePayload,
    });

    const user = userEvent.setup();
    render(<EvaluateDbTab />);
    await selectFilters(user);
    await user.click(screen.getByRole('button', { name: /Load Matches/i }));
    await screen.findByText(/Loaded 1 candidates/i, { exact: false });

    // Select and evaluate
    await user.click(screen.getByRole('radio', { name: /Select/i }));
    await user.click(screen.getByRole('button', { name: /Evaluate Selected Match/i }));

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
    expect(within(results).getAllByText(/Predicted/i).length).toBeGreaterThan(0);
    // Multiple elements may contain the word "Actual" (header and table column names)
    const actualLabels = within(results).getAllByText(/Actual\b/i);
    expect(actualLabels.length).toBeGreaterThan(0);
    expect(within(results).getByText(/Errors/i)).toBeInTheDocument();

    // Assert bowling columns appear in table (format: "{label} Actual" / "{label} Pred" / Δ)
    const table = within(results).getByRole('table');
    expect(within(table).getByText(/Wkts Pred/i)).toBeInTheDocument();
    expect(within(table).getByText(/Wkts Actual/i)).toBeInTheDocument();
    expect(within(table).getAllByTitle(/Difference/i).length).toBeGreaterThan(0);
    expect(within(table).getByText(/Econ Pred/i)).toBeInTheDocument();
    expect(within(table).getByText(/Econ Actual/i)).toBeInTheDocument();
  }, 10000);

  it('renders fielding metrics (catches, run_outs) and summary metrics when present', async () => {
    // Arrange candidates
    const backtestSelectMock = api.backtestSelect as unknown as Mock;
    backtestSelectMock.mockResolvedValue({
      filters: { format: 'T20', team1: 'IND', team2: 'AUS' },
      candidates: [
        {
          match_id: 333,
          stable_id: 'x3',
          match_date: '2024-11-06T09:00:00Z',
          venue: 'Wankhede',
          season: '2024',
          format: 'T20',
          team1: 'IND',
          team2: 'AUS',
          winner_team_code: 'IND',
        },
      ],
    });

    const fieldingResult = {
      filters: { format: 'T20', team1: 'IND', team2: 'AUS', match_id: 333 },
      match: { match_id: 333, match_date: '2024-11-06T09:00:00Z' },
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
      metrics: { player_runs_mae: 1.5, player_catches_mae: 0.5, player_run_outs_mae: 1.0 },
    };
    (api.evaluateStart as unknown as Mock).mockResolvedValue({ job_id: 'test-job-333' });
    (api.getEvaluateStatus as unknown as Mock).mockResolvedValue({
      job_id: 'test-job-333',
      match_id: '333',
      format: 'T20',
      team1: 'IND',
      team2: 'AUS',
      status: 'done',
      result: fieldingResult,
    });

    const user = userEvent.setup();
    render(<EvaluateDbTab />);
    await selectFilters(user);
    await user.click(screen.getByRole('button', { name: /Load Matches/i }));
    await screen.findByText(/Loaded 1 candidates/i, { exact: false });

    // Select and evaluate
    await user.click(screen.getByRole('radio', { name: /Select/i }));
    await user.click(screen.getByRole('button', { name: /Evaluate Selected Match/i }));

    // Assert metrics header shows fielding metrics
    const results = await screen.findByLabelText('results-section');
    expect(within(results).getByText(/player_catches_mae/i)).toBeInTheDocument();
    expect(within(results).getByText(/player_run_outs_mae/i)).toBeInTheDocument();

    // Assert fielding columns appear in table (format: "{label} Actual" / "{label} Pred" / Δ)
    const table = within(results).getByRole('table');
    expect(within(table).getByText(/Catches Pred/i)).toBeInTheDocument();
    expect(within(table).getByText(/Catches Actual/i)).toBeInTheDocument();
    expect(within(table).getAllByTitle(/Difference/i).length).toBeGreaterThan(0);
    expect(within(table).getByText(/Run Outs Pred/i)).toBeInTheDocument();
    expect(within(table).getByText(/Run Outs Actual/i)).toBeInTheDocument();
  });

  it('disable evaluate until a candidate match is selected', async () => {
    const user = userEvent.setup();
    const backtestSelectMock = api.backtestSelect as unknown as Mock;
    backtestSelectMock.mockResolvedValue({
      filters: {},
      candidates: [],
    });
    render(<EvaluateDbTab />);
    await selectFilters(user);
    await user.click(screen.getByRole('button', { name: /Load Matches/i }));
    await screen.findByText(/Loaded 0 candidates/i);
    const btn = screen.getByRole('button', {
      name: /Evaluate Selected Match/i,
    }) as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
  });
});
