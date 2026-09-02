import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import WorkbenchTab from './WorkbenchTab';

const mockXiStatus = vi.fn();
vi.mock('../api', () => ({
  api: {
    xiStatus: (...args: unknown[]) => mockXiStatus(...args),
  },
}));

const LOADED = {
  loaded: true,
  formats: ['T20', 'ODI'],
  players: 13569,
  ratings_through: '2026-08-30',
  run_id: '20260902T101500Z-ab12cd34',
  manifest: {
    run_id: '20260902T101500Z-ab12cd34',
    cutoff: '2025-09-01',
    dataset_sha: 'abc123def4567890',
    git_sha: 'deadbeefcafe',
    formats: ['T20', 'ODI'],
  },
};

describe('WorkbenchTab', () => {
  beforeEach(() => {
    mockXiStatus.mockReset();
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
    expect(screen.getByText(/Evaluation report/i)).toBeInTheDocument();
  });

  /** H-16: the Workbench answers "what is this model?" from the run's own manifest. */
  it('names the run and what its manifest recorded', async () => {
    mockXiStatus.mockResolvedValue(LOADED);
    render(<WorkbenchTab />);
    await waitFor(() => expect(mockXiStatus).toHaveBeenCalled());
    expect(await screen.findByText('20260902T101500Z-ab12cd34')).toBeInTheDocument();
    expect(screen.getByText('2025-09-01')).toBeInTheDocument();
    expect(screen.getByText('deadbee')).toBeInTheDocument();
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
});
