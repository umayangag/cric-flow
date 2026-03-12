import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import WorkbenchTab from './WorkbenchTab';

const mockGetFormats = vi.fn();
const mockAccuracyTrend = vi.fn();
const mockGetModelMetadata = vi.fn();
vi.mock('../api', () => ({
  api: {
    getFormats: (...args: unknown[]) => mockGetFormats(...args),
    accuracyTrend: (...args: unknown[]) => mockAccuracyTrend(...args),
    getModelMetadata: (...args: unknown[]) => mockGetModelMetadata(...args),
  },
}));

describe('WorkbenchTab', () => {
  beforeEach(() => {
    mockGetFormats.mockReset();
    mockAccuracyTrend.mockReset();
    mockGetModelMetadata.mockReset();
    mockGetModelMetadata.mockResolvedValue({});
  });

  it('loads formats on mount and renders format selector', async () => {
    mockGetFormats.mockResolvedValue(['T20', 'ODI']);
    render(<WorkbenchTab />);
    await waitFor(() => expect(mockGetFormats).toHaveBeenCalled());
    expect(screen.getByRole('combobox', { name: /format/i })).toBeInTheDocument();
  });

  it('renders Accuracy trend section and Load button', async () => {
    mockGetFormats.mockResolvedValue([]);
    render(<WorkbenchTab />);
    await waitFor(() => expect(mockGetFormats).toHaveBeenCalled());
    expect(screen.getByRole('button', { name: /load accuracy trend/i })).toBeInTheDocument();
  });

  it('uses backend model metadata when available', async () => {
    mockGetFormats.mockResolvedValue([]);
    mockGetModelMetadata.mockResolvedValue({
      batting: {
        level: 'player',
        hasScaler: false,
        artifactsPattern: { perFormat: 'pf', legacy: 'lg' },
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
    mockGetFormats.mockResolvedValue([]);
    mockGetModelMetadata.mockRejectedValue(new Error('Connection refused'));
    render(<WorkbenchTab />);
    await waitFor(() => expect(mockGetModelMetadata).toHaveBeenCalled());
    expect(screen.getByText(/ML service unavailable/i)).toBeInTheDocument();
    expect(screen.getByText(/Connection refused/i)).toBeInTheDocument();
  });
});
