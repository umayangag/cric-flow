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
});
