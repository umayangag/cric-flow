import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import TrackRecordTab from './TrackRecordTab';
import type { TrackRecord, TrackRecordEntry } from '../types';

const mockTrackRecord = vi.fn();
const mockEvaluationReport = vi.fn();
vi.mock('../api', () => ({
  api: {
    trackRecord: (...args: unknown[]) => mockTrackRecord(...args),
    evaluationReport: (...args: unknown[]) => mockEvaluationReport(...args),
  },
}));

function entry(overrides: Partial<TrackRecordEntry>): TrackRecordEntry {
  return {
    id: overrides.id ?? 'p-1',
    issued_at: '2026-09-07T10:00:00Z',
    run_id: '20260907T062657Z-6b16045e',
    ratings_through: '2026-09-02',
    format: 'T20I',
    gender: 'male',
    match_date: '2026-09-10',
    team1: { id: 4, name: 'Australia (men)' },
    team2: { id: 54, name: 'England (men)' },
    objective: 'win',
    state: 'unresolved',
    population: 'with_shared_factor',
    claimed: {
      win_probability_team1: 0.317,
      win_probability_source: 'display',
      predicted_winner_id: 54,
      team1_range: { p10: 140, p90: 190 },
      team2_range: { p10: 135, p90: 185 },
      team1_players: 11,
      team2_players: 11,
    },
    ...overrides,
  };
}

function emptyRecord(overrides: Partial<TrackRecord> = {}): TrackRecord {
  return {
    computed_at: '2026-09-07T12:00:00Z',
    today: '2026-09-07',
    total: 0,
    states: { scenario: 0, superseded: 0, unresolved: 0, no_result: 0, scored: 0 },
    win: {
      overall: { n: 0, brier: null, base_rate: null, base_rate_brier: null },
      by_format: {},
      reliability: [],
      reliability_bins: 10,
    },
    coverage: {
      rows: [],
      populations: {
        with_shared_factor: 0,
        without_shared_factor: 0,
        unknown: 0,
        not_simulated: 0,
      },
    },
    elevens: { n: 0, mean_overlap: null, min_overlap: null, max_overlap: null, complete: 0 },
    predictions: [],
    ...overrides,
  };
}

/** One prediction in every state, and the summaries a scored one produces. */
function fullRecord(): TrackRecord {
  return emptyRecord({
    total: 5,
    states: { scenario: 1, superseded: 1, unresolved: 1, no_result: 1, scored: 1 },
    win: {
      overall: { n: 1, brier: 0.1, base_rate: 0, base_rate_brier: 0 },
      by_format: { T20I: { n: 1, brier: 0.1, base_rate: 0, base_rate_brier: 0 } },
      reliability: [{ lo: 0.3, hi: 0.4, n: 1, predicted: 0.317, observed: 0 }],
      reliability_bins: 10,
    },
    coverage: {
      rows: [
        {
          format: 'T20I',
          population: 'with_shared_factor',
          n_predictions: 1,
          first_innings: { n: 1, covered: 0, coverage: 0 },
          chase: { n: 1, covered: 1, coverage: 1 },
        },
      ],
      populations: {
        with_shared_factor: 1,
        without_shared_factor: 0,
        unknown: 0,
        not_simulated: 0,
      },
    },
    elevens: { n: 1, mean_overlap: 20, min_overlap: 20, max_overlap: 20, complete: 0 },
    predictions: [
      entry({
        id: 'scored',
        state: 'scored',
        happened: {
          match_id: 1,
          winner_opposition_id: 54,
          team1_total: 201,
          team2_total: 160,
          team1_batted_first: true,
        },
        score: {
          team1_won: false,
          brier: 0.1,
          team1_covered: false,
          team2_covered: true,
          eleven_overlap: { matched: 20, of: 22, team1_matched: 10, team2_matched: 10 },
        },
      }),
      entry({
        id: 'no-result',
        state: 'no_result',
        happened: {
          match_id: 2,
          winner_opposition_id: null,
          team1_total: 40,
          team2_total: null,
          team1_batted_first: true,
        },
      }),
      entry({ id: 'unresolved', state: 'unresolved', days_past_match_date: -3 }),
      entry({
        id: 'superseded',
        state: 'superseded',
        superseded_by: 'scored',
        state_note: 'a later forecast of the same fixture was issued 2026-09-07T10:00:00Z',
      }),
      entry({ id: 'scenario', state: 'scenario', objective: 'fixed' }),
    ],
  });
}

describe('TrackRecordTab', () => {
  beforeEach(() => {
    mockTrackRecord.mockReset();
    mockEvaluationReport.mockReset();
    mockEvaluationReport.mockRejectedValue(new Error('no evaluation report'));
  });

  it('shows every prediction in its state, one row each, with the state counts', async () => {
    mockTrackRecord.mockResolvedValue(fullRecord());
    render(<TrackRecordTab />);

    const rows = await screen.findAllByTestId('track-record-row');

    expect(rows.map((row) => row.dataset.state)).toEqual([
      'scored',
      'no_result',
      'unresolved',
      'superseded',
      'scenario',
    ]);
    expect(screen.getByTestId('state-count-scored')).toHaveTextContent('scored 1');
    expect(screen.getByTestId('state-count-scenario')).toHaveTextContent('scenario 1');
    expect(screen.getByTestId('state-count-no_result')).toHaveTextContent('no result 1');
    expect(within(rows[2]).getByText('match in 3 days')).toBeInTheDocument();
    expect(
      within(rows[4]).getByText('hand-built eleven; listed, never scored'),
    ).toBeInTheDocument();
    expect(within(rows[0]).getByText('England (men) won')).toBeInTheDocument();
  });

  it('carries n on every summary number and keeps the miss on the record', async () => {
    mockTrackRecord.mockResolvedValue(fullRecord());
    render(<TrackRecordTab />);

    await screen.findAllByTestId('track-record-row');

    const win = screen.getByRole('table', { name: 'win probability scores' });
    expect(within(win).getByText('All scored predictions').closest('tr')).toHaveTextContent('1');
    expect(within(win).getAllByText('0.100')).toHaveLength(2);
    const coverage = screen.getByRole('table', { name: 'coverage by population' });
    expect(within(coverage).getByText('0 of 1 (0.0%)')).toBeInTheDocument();
    expect(within(coverage).getByText('1 of 1 (100.0%)')).toBeInTheDocument();
    expect(screen.getByTestId('population-count-with_shared_factor')).toHaveTextContent(
      'shared factor 1',
    );
    expect(screen.getByTestId('population-count-unknown')).toHaveTextContent('unknown simulator 0');
    expect(screen.getByTestId('elevens-summary')).toHaveTextContent('Over 1 scored prediction');
    expect(screen.getByTestId('elevens-summary')).toHaveTextContent(
      '0 had every named player play',
    );
    expect(screen.getByTestId('reliability-plot')).toHaveAttribute(
      'aria-label',
      'reliability plot, 1 of 10 bins populated, n 1',
    );
    expect(screen.getAllByTestId('reliability-point')).toHaveLength(1);
    expect(screen.getByText(/the harness columns are empty/)).toBeInTheDocument();
  });

  it('renders an empty record without inventing a curve', async () => {
    mockTrackRecord.mockResolvedValue(emptyRecord());
    render(<TrackRecordTab />);

    const plot = await screen.findByTestId('reliability-plot');

    expect(screen.queryAllByTestId('reliability-point')).toHaveLength(0);
    expect(within(plot).getByText('no scored predictions yet')).toBeInTheDocument();
    expect(plot).toHaveAttribute('aria-label', 'reliability plot, 0 of 10 bins populated, n 0');
    expect(screen.getByText('No scored prediction with a served range yet.')).toBeInTheDocument();
    expect(screen.getByTestId('elevens-summary')).toHaveTextContent('n=0');
    expect(screen.getByText(/Nothing on the record yet/)).toBeInTheDocument();
    expect(screen.getByTestId('state-count-unresolved')).toHaveTextContent('unresolved 0');
  });

  it('shows the harness figures beside the record where the report has them', async () => {
    mockTrackRecord.mockResolvedValue(fullRecord());
    mockEvaluationReport.mockResolvedValue({
      generated_at: '2026-09-02T19:59:13',
      formats: {
        T20I: {
          n_matches: 100,
          locked: {
            cutoff: '2026-09-02',
            end: '9999',
            n_eval: 0,
            n_train: 100,
            skipped_reason: 'no matches',
          },
          walk_forward: {
            folds: [],
            summary: {
              base_rate_brier: { mean: 0.25, sd: 0.001, n_folds: 10 },
              simulation: {
                n_matches: { mean: 40, sd: 2, n_folds: 10 },
                win: {
                  brier: { display: { mean: 0.2036, sd: 0.01, n_folds: 10 }, simulated: 0.2 },
                },
                totals: {
                  first_innings: { coverage_80: { mean: 0.783, sd: 0.05, n_folds: 10 } },
                  chase: { coverage_80: { mean: 0.688, sd: 0.05, n_folds: 10 } },
                },
                shared_factor_folds: {
                  folds_scored: 10,
                  with_shared_factor: 9,
                  without_shared_factor: 1,
                  windows_without_shared_factor: ['2025-06-01'],
                  totals_with_shared_factor: {
                    first_innings: { coverage_80: { mean: 0.792, sd: 0.05, n_folds: 9 } },
                    chase: { coverage_80: { mean: 0.692, sd: 0.05, n_folds: 9 } },
                  },
                },
              },
            },
          },
          simulation_decision: { simulated_win_probability_within_tolerance: true },
        },
      },
    });
    render(<TrackRecordTab />);

    await screen.findAllByTestId('track-record-row');

    const win = screen.getByRole('table', { name: 'win probability scores' });
    const formatRow = within(win).getByText('T20I').closest('tr');
    expect(formatRow).toHaveTextContent('0.204');
    expect(formatRow).toHaveTextContent('0.250');
    expect(formatRow).toHaveTextContent('walk-forward fold means, n=40');
    const coverageRow = screen.getAllByTestId('coverage-row')[0];
    expect(coverageRow).toHaveTextContent('79.2%');
    expect(coverageRow).toHaveTextContent('69.2%');
    expect(coverageRow).toHaveTextContent('folds with a shared factor');
    expect(screen.queryByText(/the harness columns are empty/)).not.toBeInTheDocument();
  });

  it('shows the failure and a retry when the record cannot be read', async () => {
    mockTrackRecord.mockRejectedValue(new Error('db pool not initialized'));
    render(<TrackRecordTab />);

    await waitFor(() => expect(screen.getByText('No track record')).toBeInTheDocument());

    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument();
  });
});
