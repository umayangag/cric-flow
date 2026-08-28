import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import AccuracyTrendRuns from './AccuracyTrendRuns';
import type { AccuracyTrendItem, Migration } from '../types';

const results: AccuracyTrendItem[] = [
  {
    match_id: 1,
    match_date: '2026-06-01',
    format: 'T20I',
    team1: 'IND',
    team2: 'AUS',
    metrics: {},
  },
  {
    match_id: 2,
    match_date: '2026-08-01',
    format: 'T20I',
    team1: 'IND',
    team2: 'AUS',
    metrics: {},
  },
];

function migration(over: Partial<Migration>): Migration {
  return {
    id: 1,
    command: 'train-batting',
    args: null,
    started_at: '2026-07-01T00:00:00Z',
    status: 'COMPLETED',
    ...over,
  };
}

describe('AccuracyTrendRuns', () => {
  it('lists runs inside the plotted window, newest first', () => {
    render(
      <AccuracyTrendRuns
        results={results}
        migrations={[
          migration({ id: 1, command: 'train-batting', started_at: '2026-06-15T00:00:00Z' }),
          migration({ id: 2, command: 'ml-auto-tune', started_at: '2026-07-20T00:00:00Z' }),
        ]}
        loading={false}
      />,
    );
    const rows = screen.getAllByRole('row').slice(1);
    expect(rows[0]).toHaveTextContent('auto-tune');
    expect(rows[1]).toHaveTextContent('retrain');
  });

  it('ignores runs outside the window — they cannot explain movement inside it', () => {
    render(
      <AccuracyTrendRuns
        results={results}
        migrations={[migration({ started_at: '2025-01-01T00:00:00Z' })]}
        loading={false}
      />,
    );
    expect(screen.getByText(/No retrain, auto-tune or dataset change ran/)).toBeInTheDocument();
  });

  it('ignores runs that cannot change what a prediction says', () => {
    render(
      <AccuracyTrendRuns
        results={results}
        migrations={[migration({ command: 'precompute-features' })]}
        loading={false}
      />,
    );
    expect(screen.getByText(/No retrain, auto-tune or dataset change ran/)).toBeInTheDocument();
  });

  it('ignores a run that failed', () => {
    render(
      <AccuracyTrendRuns
        results={results}
        migrations={[migration({ status: 'FAILED' })]}
        loading={false}
      />,
    );
    expect(screen.getByText(/No retrain, auto-tune or dataset change ran/)).toBeInTheDocument();
  });

  it('renders nothing when there is no trend to annotate', () => {
    const { container } = render(
      <AccuracyTrendRuns results={[]} migrations={[migration({})]} loading={false} />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
