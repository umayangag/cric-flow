import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import HealthTab from './HealthTab';

const mockApiHealth = vi.fn();
const mockHealth = vi.fn();
vi.mock('../api', () => ({
  api: {
    apiHealth: (...args: unknown[]) => mockApiHealth(...args),
    health: (...args: unknown[]) => mockHealth(...args),
  },
}));

describe('HealthTab', () => {
  beforeEach(() => {
    mockApiHealth.mockReset();
    mockHealth.mockReset();
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
});
