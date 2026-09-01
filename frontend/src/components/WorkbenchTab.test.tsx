import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import WorkbenchTab from './WorkbenchTab';

const mockGetModelMetadata = vi.fn();
const mockGetModelStats = vi.fn();
vi.mock('../api', () => ({
  api: {
    getModelMetadata: (...args: unknown[]) => mockGetModelMetadata(...args),
    getModelStats: (...args: unknown[]) => mockGetModelStats(...args),
  },
}));

describe('WorkbenchTab', () => {
  beforeEach(() => {
    mockGetModelMetadata.mockReset();
    mockGetModelStats.mockReset();
    mockGetModelMetadata.mockResolvedValue({});
    mockGetModelStats.mockResolvedValue({ models: [] });
  });

  it('points at the evaluation report rather than re-scoring matches here', async () => {
    render(<WorkbenchTab />);
    await waitFor(() => expect(mockGetModelMetadata).toHaveBeenCalled());
    expect(screen.getByText(/Evaluation report/i)).toBeInTheDocument();
  });

  it('uses backend model metadata when available', async () => {
    mockGetModelMetadata.mockResolvedValue({
      batting: {
        level: 'player',
        hasScaler: false,
        artifactsPattern: { perFormat: 'pf' },
        features: ['x_feature'],
        outputs: ['y_output'],
        note: 'from-backend',
      },
    });
    render(<WorkbenchTab />);
    await waitFor(() => expect(mockGetModelMetadata).toHaveBeenCalled());
    expect(screen.getByText(/from-backend/i)).toBeInTheDocument();
  });

  it('shows error when ML service is down (model metadata fetch fails)', async () => {
    mockGetModelMetadata.mockRejectedValue(new Error('Connection refused'));
    render(<WorkbenchTab />);
    await waitFor(() => expect(mockGetModelMetadata).toHaveBeenCalled());
    expect(screen.getByText(/ML service unavailable/i)).toBeInTheDocument();
    expect(screen.getByText(/Connection refused/i)).toBeInTheDocument();
  });
});
