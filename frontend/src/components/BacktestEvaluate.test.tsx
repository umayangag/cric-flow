import React from 'react';
import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { BacktestEvaluate } from './BacktestEvaluate';

// Mock API client evaluate helper
vi.mock('../api/client', () => ({
  fetchBacktestEvaluate: vi.fn(),
}));

import { fetchBacktestEvaluate } from '../api/client';

describe('BacktestEvaluate component', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('validates RFC3339 cutoff and shows error when invalid', async () => {
    const onResult = vi.fn();
    render(
      <BacktestEvaluate matchId={789} format="T20" team1="IND" team2="AUS" onResult={onResult} />,
    );
    // Leave cutoff empty and click evaluate
    fireEvent.click(screen.getByLabelText('evaluate'));
    expect(await screen.findByRole('alert')).toHaveTextContent(/cutoff/i);
    expect(onResult).not.toHaveBeenCalled();
  });

  it('calls evaluate and renders summary on success', async () => {
    const onResult = vi.fn();
    const mockedFetch = fetchBacktestEvaluate as unknown as Mock;
    mockedFetch.mockResolvedValue({
      filters: { delegated: true, model_version: 'v-test' },
      match: { match_id: 789, date: '2024-10-30T14:00:00Z' },
      match_aggregates: {
        predicted: { runs: 160, wickets: 6, extras: 10, winner_team_code: 'IND' },
        actual: { runs: 155, wickets: 7, extras: 12, winner_team_code: 'IND' },
        errors: { runs_mae: 5 },
      },
      players: [
        { player_id: 101, predicted: { runs: 20 }, actual: { runs: 18 }, errors: { runs_mae: 2 } },
      ],
      metrics: { player_runs_mae: 2, winner_accuracy: 1 },
    });

    render(
      <BacktestEvaluate matchId={789} format="T20" team1="IND" team2="AUS" onResult={onResult} />,
    );
    const cutoff = screen.getByLabelText('cutoff');
    fireEvent.change(cutoff, { target: { value: '2024-10-30T14:00:00Z' } });
    fireEvent.click(screen.getByLabelText('evaluate'));

    await waitFor(() => expect(fetchBacktestEvaluate).toHaveBeenCalledTimes(1));
    // Summary renders
    const summary = await screen.findByLabelText('evaluation-summary');
    expect(summary).toBeInTheDocument();
    expect(summary.textContent || '').toMatch(/player_runs_mae/i);
    expect(summary.textContent || '').toMatch(/runs \(pred vs actual\)/i);
    expect(onResult).toHaveBeenCalledTimes(1);
  });
});
