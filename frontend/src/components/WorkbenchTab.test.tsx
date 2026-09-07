import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import WorkbenchTab from './WorkbenchTab';

const mockXiStatus = vi.fn();
const mockOpsStatus = vi.fn();
vi.mock('../api', () => ({
  api: {
    xiStatus: (...args: unknown[]) => mockXiStatus(...args),
    opsStatus: (...args: unknown[]) => mockOpsStatus(...args),
  },
}));

/** /ops/status with the one freshness object the loaded-run card reads (P2-1). */
const OPS_STATUS_FRESH = {
  timestamp: '2026-09-02T10:00:00Z',
  freshness: {
    served: {
      status: 'fresh',
      fresh: true,
      age_days: 3,
      max_age_days: 14,
      ratings_through: '2026-08-30',
      code: null,
    },
    database: {},
    retrain_due: { status: 'up_to_date', days_behind: 0, latest_match_date: '2026-08-30' },
  },
};

const LOADED = {
  loaded: true,
  formats: ['T20', 'ODI'],
  players: 13569,
  ratings_through: '2026-08-30',
  run_id: '20260902T101500Z-ab12cd34',
  manifest: {
    run_id: '20260902T101500Z-ab12cd34',
    cutoff: '2025-09-01',
    ratings_through: '2026-08-30',
    dataset_sha: 'abc123def4567890',
    git_sha: 'deadbeefcafe',
    formats: ['T20', 'ODI'],
    metrics: {
      T20: { objective_auc: 0.723, display_auc_mean: 0.711 },
      ODI: { objective_auc: 0.702, display_auc_mean: 0.698 },
    },
  },
};

describe('WorkbenchTab', () => {
  beforeEach(() => {
    mockXiStatus.mockReset();
    mockOpsStatus.mockReset();
    mockOpsStatus.mockResolvedValue(OPS_STATUS_FRESH);
    mockXiStatus.mockResolvedValue({
      loaded: false,
      formats: [],
      players: 0,
      ratings_through: null,
    });
  });

  it('points at the evaluation report rather than re-scoring matches here', async () => {
    render(<WorkbenchTab />);
    await waitFor(() => expect(mockXiStatus).toHaveBeenCalled());
    expect(screen.getAllByText(/Evaluation report/i).length).toBeGreaterThan(0);
  });

  /** H-16: the Workbench answers "what is this model?" from the run's own manifest. */
  it('names the run and what its manifest recorded', async () => {
    mockXiStatus.mockResolvedValue(LOADED);
    render(<WorkbenchTab />);
    await waitFor(() => expect(mockXiStatus).toHaveBeenCalled());
    expect(await screen.findByText('20260902T101500Z-ab12cd34')).toBeInTheDocument();
    expect(screen.getByText('2025-09-01')).toBeInTheDocument();
    expect(screen.getByText('deadbee')).toBeInTheDocument();
    // The manifest's own date beside the cutoff (P2-2), and the verdict reads the same one.
    expect(screen.getByText('Ratings through (manifest)')).toBeInTheDocument();
    expect(screen.getByText('2026-08-30')).toBeInTheDocument();
    expect(screen.getByText(/ratings through 2026-08-30 \(3 days old/)).toBeInTheDocument();
  });

  /** L-1: the run's headline metrics are a table keyed by metric, not a JSON dump. */
  it('shows the run headline metrics per format, by their metric keys', async () => {
    mockXiStatus.mockResolvedValue(LOADED);
    render(<WorkbenchTab />);
    await waitFor(() => expect(mockXiStatus).toHaveBeenCalled());

    expect(await screen.findByText('objective_auc')).toBeInTheDocument();
    expect(screen.getByText('display_auc_mean')).toBeInTheDocument();
    expect(screen.getByText('0.7230')).toBeInTheDocument();
    expect(screen.getByText('0.6980')).toBeInTheDocument();
  });

  /** B-3: a run with no holdout still trained its formats, and the manifest says why the
   * discrimination numbers are missing rather than rendering an empty table. */
  it('says why a format carries no headline metrics', async () => {
    mockXiStatus.mockResolvedValue({
      ...LOADED,
      manifest: {
        ...LOADED.manifest,
        metrics: { T20: { n_train: 21096, n_holdout: 0 } },
        format_notes: {
          T20: 'trained on 21096 rows but not scored: holdout too small or single-class; no discrimination numbers (0 rows at or after the cutoff)',
        },
      },
    });
    render(<WorkbenchTab />);
    await waitFor(() => expect(mockXiStatus).toHaveBeenCalled());

    expect(await screen.findByText(/0 rows at or after the cutoff/)).toBeInTheDocument();
    expect(screen.getByText('n_holdout')).toBeInTheDocument();
  });

  /** D-6: a refused artifact set has to read as refused, not as "nothing trained yet". */
  it('says why nothing is loaded when the artifacts were refused', async () => {
    mockXiStatus.mockResolvedValue({
      loaded: false,
      formats: [],
      players: 0,
      ratings_through: null,
      error: 'run 20260902T101500Z-ab12cd34: bat_pos_sum has width 1024, expected 13427',
    });
    render(<WorkbenchTab />);
    await waitFor(() => expect(mockXiStatus).toHaveBeenCalled());
    expect(await screen.findByText(/expected 13427/)).toBeInTheDocument();
  });

  it('shows the error when the ML service is unreachable', async () => {
    mockXiStatus.mockRejectedValue(new Error('Connection refused'));
    render(<WorkbenchTab />);
    await waitFor(() => expect(mockXiStatus).toHaveBeenCalled());
    expect(await screen.findByText(/Connection refused/i)).toBeInTheDocument();
  });

  /**
   * P2-1: the card's ratings line is the one freshness object's verdict, in the same
   * words the Health tab and the Ops badge use — not a date this card reads for itself
   * off the run status beside it.
   */
  it('shows the served verdict from the one freshness object', async () => {
    mockXiStatus.mockResolvedValue(LOADED);
    render(<WorkbenchTab />);
    expect(
      await screen.findByText(/ratings through 2026-08-30 \(3 days old, limit 14\)/),
    ).toBeInTheDocument();
  });
});
