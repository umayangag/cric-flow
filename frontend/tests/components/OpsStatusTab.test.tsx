import { render, screen, waitFor, act, cleanup } from '@testing-library/react';
import { vi } from 'vitest';
import React from 'react';
import OpsStatusTab from '../../src/components/OpsStatusTab';

// Mock the API module used by the component
const mockOpsStatus = vi.fn();
vi.mock('../../src/api', () => ({
  api: {
    opsStatus: (...args: any[]) => mockOpsStatus(...args),
  },
}));

describe('OpsStatusTab', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mockOpsStatus.mockReset();
  });
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it('fetches status, renders sections, and sets up auto-refresh interval', async () => {
    mockOpsStatus.mockResolvedValueOnce({
      timestamp: '2026-01-22T10:00:00Z',
      services: { api_health: true, api_readiness: true, ml_health: true },
      db: { connected: true },
      precompute: { formats: { TEST: { status: 'ok' } } },
      exports: { formats: { TEST: { files: [] } } },
      artifacts: { formats: { TEST: { batting: { exists: false }, bowling: { exists: false } } } },
      suggestions: [],
    });

    render(<OpsStatusTab />);

    // First fetch
    await waitFor(() => expect(mockOpsStatus).toHaveBeenCalledTimes(1));

    // Sections should render
    expect(screen.getByText(/Services/i)).toBeInTheDocument();
    expect(screen.getByText(/Database/i)).toBeInTheDocument();
    expect(screen.getByText(/Precompute/i)).toBeInTheDocument();
    expect(screen.getByText(/Exports/i)).toBeInTheDocument();
    expect(screen.getByText(/Artifacts/i)).toBeInTheDocument();
    expect(screen.getByText(/Suggestions/i)).toBeInTheDocument();

    // Advance timer to trigger interval refresh
    act(() => {
      vi.advanceTimersByTime(15000);
    });

    await waitFor(() => expect(mockOpsStatus).toHaveBeenCalledTimes(2));
  });

  it('shows error when fetch fails and clears interval on unmount', async () => {
    const clearSpy = vi.spyOn(globalThis, 'clearInterval');
    mockOpsStatus.mockRejectedValueOnce(new Error('boom'));

    const { unmount } = render(<OpsStatusTab />);

    // Error message should be displayed
    await waitFor(() => expect(screen.getByText(/Error:/i)).toBeInTheDocument());

    // Unmount should clear the interval
    unmount();
    expect(clearSpy).toHaveBeenCalled();
    clearSpy.mockRestore();
  });
});
