import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import UpcomingMatchTab from './UpcomingMatchTab';

const mockGetFormats = vi.fn();
vi.mock('../api', () => ({
  api: {
    getFormats: (...args: unknown[]) => mockGetFormats(...args),
  },
}));

describe('UpcomingMatchTab', () => {
  beforeEach(() => {
    mockGetFormats.mockReset();
  });

  it('loads formats on mount and renders format selector', async () => {
    mockGetFormats.mockResolvedValue(['T20', 'ODI']);
    render(<UpcomingMatchTab />);
    await waitFor(() => expect(mockGetFormats).toHaveBeenCalled());
    expect(screen.getByRole('heading', { name: /upcoming match prediction/i })).toBeInTheDocument();
    expect(screen.getAllByRole('combobox').length).toBeGreaterThan(0);
  });

  it('renders team and date inputs', async () => {
    mockGetFormats.mockResolvedValue([]);
    render(<UpcomingMatchTab />);
    await waitFor(() => expect(mockGetFormats).toHaveBeenCalled());
    expect(screen.getByRole('heading', { name: /upcoming match prediction/i })).toBeInTheDocument();
    expect(screen.getAllByPlaceholderText(/select or type team/i).length).toBe(2);
  });
});
