import React from 'react';
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { BacktestResults } from '../../../src/components/BacktestResults';

describe('BacktestResults component', () => {
  it('renders metrics, match aggregates, and players table', async () => {
    const result = {
      filters: { delegated: true, model_version: 'v-test' },
      match: { match_id: 789, date: '2024-10-30T14:00:00Z' },
      match_aggregates: {
        predicted: { runs: 160, wickets: 6, extras: 10, winner_team_code: 'IND' },
        actual: { runs: 155, wickets: 7, extras: 12, winner_team_code: 'IND' },
        errors: { runs_mae: 5, wickets_mae: 1, extras_mae: 2 },
      },
      players: [
        { player_id: 101, predicted: { runs: 20 }, actual: { runs: 18 }, errors: { runs_mae: 2 } },
        { player_id: 102, predicted: { runs: 0 }, actual: { runs: 5 }, errors: { runs_mae: 5 } },
      ],
      metrics: { player_runs_mae: 3.5, player_runs_rmse: 4.2, winner_accuracy: 1 },
    } as any;

    render(<BacktestResults result={result} />);

    const metrics = await screen.findByLabelText('metrics-summary');
    expect(metrics.textContent || '').toMatch(/player_runs_mae: 3.5/);
    expect(metrics.textContent || '').toMatch(/player_runs_rmse: 4.2/);
    expect(metrics.textContent || '').toMatch(/winner_accuracy: 1/);
    expect(metrics.textContent || '').toMatch(/model_version: v-test/);

    const ma = await screen.findByLabelText('match-aggregates');
    expect(ma.textContent || '').toMatch(/Predicted/);
    expect(ma.textContent || '').toMatch(/Actual/);
    expect(ma.textContent || '').toMatch(/Errors/);
    expect(ma.textContent || '').toMatch(/runs: 160/);
    expect(ma.textContent || '').toMatch(/runs: 155/);
    expect(ma.textContent || '').toMatch(/runs_mae: 5/);

    const table = await screen.findByRole('table', { name: /players-table/i });
    expect(table).toBeInTheDocument();
    expect(table.textContent || '').toMatch(/101/);
    expect(table.textContent || '').toMatch(/102/);
    expect(table.textContent || '').toMatch(/20/);
    expect(table.textContent || '').toMatch(/18/);
  });
});
