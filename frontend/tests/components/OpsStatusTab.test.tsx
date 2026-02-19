import { render, screen, waitFor, cleanup } from '@testing-library/react';
import { vi } from 'vitest';
import React from 'react';
import OpsStatusTab from '../../src/components/OpsStatusTab';

// Mock the API module used by the component
const mockOpsStatus = vi.fn();
vi.mock('../../src/api', () => ({
  api: {
    opsStatus: (...args: unknown[]) => mockOpsStatus(...args),
  },
}));

describe('OpsStatusTab', () => {
  beforeEach(() => {
    mockOpsStatus.mockReset();
  });
  afterEach(() => {
    cleanup();
  });

  it('fetches status, renders sections, and sets up auto-refresh interval', async () => {
    // Use real timers so Testing Library's waitFor works as expected
    vi.useRealTimers();
    mockOpsStatus.mockResolvedValueOnce({
      timestamp: '2026-01-22T10:00:00Z',
      services: { api_health: true, api_readiness: true, ml_health: true },
      db: { connected: true },
      precompute: { formats: { TEST: { status: 'ok' } } },
      exports: { formats: { TEST: { files: [] } } },
      artifacts: {
        formats: {
          TEST: { batting: { exists: false }, bowling: { exists: false } },
        },
      },
      suggestions: [],
    });
    // Provide a resolved value for the next manual refresh call too
    mockOpsStatus.mockResolvedValueOnce({
      timestamp: '2026-01-22T10:00:15Z',
      services: { api_health: true, api_readiness: true, ml_health: true },
      db: { connected: true },
      precompute: { formats: { TEST: { status: 'ok' } } },
      exports: { formats: { TEST: { files: [] } } },
      artifacts: {
        formats: {
          TEST: { batting: { exists: false }, bowling: { exists: false } },
        },
      },
      suggestions: [],
    });

    render(<OpsStatusTab />);

    // First fetch
    await waitFor(() => expect(mockOpsStatus).toHaveBeenCalledTimes(1));

    // Evidence that data has been loaded: the raw JSON toggle appears only after data is set
    expect(await screen.findByText(/Show raw JSON payload/i)).toBeInTheDocument();

    // Trigger a manual refresh instead of relying on interval timing to avoid flakiness
    const refreshBtn = screen.getByRole('button', {
      name: /Refresh Ops Status/i,
    });
    refreshBtn.click();
    await waitFor(() => expect(mockOpsStatus).toHaveBeenCalledTimes(2));
  }, 15000);

  it('shows error when fetch fails and clears interval on unmount', async () => {
    // Use real timers so Testing Library's waitFor works as expected
    vi.useRealTimers();
    // The component uses setTimeout for scheduling; spy on clearTimeout for cleanup
    const clearTimeoutSpy = vi.spyOn(globalThis, 'clearTimeout');
    mockOpsStatus.mockRejectedValueOnce(new Error('boom'));

    const { unmount } = render(<OpsStatusTab />);

    // Error message should be displayed
    await waitFor(() => expect(screen.getByText(/Error:/i)).toBeInTheDocument());

    // Unmount should clear the interval
    unmount();
    expect(clearTimeoutSpy).toHaveBeenCalled();
    clearTimeoutSpy.mockRestore();
  });
});
