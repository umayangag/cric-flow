import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import HealthTab from './HealthTab';

const mockApiHealth = vi.fn();
const mockHealth = vi.fn();
const mockOpsStatus = vi.fn();
vi.mock('../api', () => ({
  api: {
    apiHealth: (...args: unknown[]) => mockApiHealth(...args),
    health: (...args: unknown[]) => mockHealth(...args),
    opsStatus: (...args: unknown[]) => mockOpsStatus(...args),
  },
}));

/** /ops/status with the one freshness object the Ratings line reads (P2-1). */
function opsStatusWith(served: Record<string, unknown>) {
  return {
    timestamp: '2026-09-07T00:00:00Z',
    freshness: {
      served,
      database: {},
      retrain_due: { status: 'unknown', days_behind: null, latest_match_date: null },
    },
  };
}

describe('HealthTab', () => {
  beforeEach(() => {
    mockApiHealth.mockReset();
    mockHealth.mockReset();
    mockOpsStatus.mockReset();
    mockOpsStatus.mockResolvedValue(
      opsStatusWith({
        status: 'fresh',
        fresh: true,
        data_age_days: 1,
        max_age_days: 14,
        data_through: '2026-09-06',
        ratings_through: '2026-09-05',
        code: null,
      }),
    );
  });

  it('renders Refresh button and fetches health on mount', async () => {
    mockApiHealth.mockResolvedValue('ok');
    mockHealth.mockResolvedValue({
      status: 'ok',
      loaded_batting_formats: [],
      loaded_bowling_formats: [],
    });
    render(<HealthTab />);
    expect(screen.getByRole('button', { name: /refresh/i })).toBeInTheDocument();
    await waitFor(() => {
      expect(mockApiHealth).toHaveBeenCalled();
      expect(mockHealth).toHaveBeenCalled();
    });
    await waitFor(() => {
      expect(screen.getByText(/go api/i)).toBeInTheDocument();
      expect(screen.getByText(/ml service/i)).toBeInTheDocument();
    });
  });

  it('shows error when both health checks fail', async () => {
    mockApiHealth.mockRejectedValue(new Error('Network error'));
    mockHealth.mockRejectedValue(new Error('Network error'));
    mockOpsStatus.mockRejectedValue(new Error('Network error'));
    render(<HealthTab />);
    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(/both api and ml health checks failed/i);
    });
  });

  it('calling Refresh re-fetches health', async () => {
    mockApiHealth.mockResolvedValue('ok');
    mockHealth.mockResolvedValue({ status: 'ok' });
    const user = userEvent.setup();
    render(<HealthTab />);
    await waitFor(() => expect(mockApiHealth).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole('button', { name: /refresh/i }));
    await waitFor(() => expect(mockApiHealth).toHaveBeenCalledTimes(2));
  });

  /**
   * The Ratings line is the one freshness object's, not this tab's own reading of the ML
   * health payload: one verdict, assembled once, rendered in the same words everywhere
   * (P2-1).
   */
  it('shows the served verdict from the one freshness object', async () => {
    mockApiHealth.mockResolvedValue({ status: 'ok' });
    mockHealth.mockResolvedValue({ status: 'ok', run_id: 'r1', loaded_xi_formats: ['T20'] });
    render(<HealthTab />);
    await waitFor(() => {
      expect(
        screen.getByText(
          /data built to 2026-09-06 \(1 days ago, limit 14\); last match 2026-09-05/,
        ),
      ).toBeInTheDocument();
    });
  });

  it('says the verdict is unknown when /ops/status did not answer', async () => {
    mockApiHealth.mockResolvedValue({ status: 'ok' });
    mockHealth.mockResolvedValue({ status: 'ok', run_id: 'r1' });
    mockOpsStatus.mockRejectedValue(new Error('Network error'));
    render(<HealthTab />);
    await waitFor(() => {
      expect(screen.getByText(/ratings date is unknown/)).toBeInTheDocument();
    });
  });
});
