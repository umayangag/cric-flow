import React from 'react';
import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { BacktestFilters } from './BacktestFilters';

// Mock API client (avoid any)
vi.mock('../api/client', () => ({
  fetchBacktestSelect: vi.fn(),
}));

import { fetchBacktestSelect } from '../api/client';

describe('BacktestFilters component', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders inputs and search button', () => {
    const onSelect = vi.fn();
    render(<BacktestFilters onSelect={onSelect} />);
    expect(screen.getByLabelText('format')).toBeInTheDocument();
    expect(screen.getByLabelText('team1')).toBeInTheDocument();
    expect(screen.getByLabelText('team2')).toBeInTheDocument();
    expect(screen.getByLabelText('search')).toBeInTheDocument();
  });

  it('shows validation error when required fields missing', async () => {
    const onSelect = vi.fn();
    render(<BacktestFilters onSelect={onSelect} />);
    const team1 = screen.getByLabelText('team1');
    fireEvent.change(team1, { target: { value: '' } });
    fireEvent.click(screen.getByLabelText('search'));
    expect(await screen.findByRole('alert')).toHaveTextContent(/required/i);
  });

  it('fetches candidates and invokes onSelect on click', async () => {
    const onSelect = vi.fn();
    const mockedFetch = fetchBacktestSelect as unknown as Mock;
    mockedFetch.mockResolvedValue({
      filters: {},
      candidates: [
        {
          match_id: 9000111,
          stable_id: 'm-1',
          date: '2024-10-30T14:00:00Z',
          venue: 'Ground',
          season: '2024',
          format: 'T20',
          team1: 'IND',
          team2: 'AUS',
          winner_team_code: 'IND',
        },
      ],
    });

    render(<BacktestFilters onSelect={onSelect} />);
    fireEvent.click(screen.getByLabelText('search'));

    await waitFor(() => expect(fetchBacktestSelect).toHaveBeenCalledTimes(1));
    const table = await screen.findByRole('table', { name: /candidates-table/i });
    expect(table).toBeInTheDocument();
    const selectBtn = await screen.findByLabelText('select-9000111');
    fireEvent.click(selectBtn);
    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(onSelect).toHaveBeenCalledWith(9000111, expect.objectContaining({ match_id: 9000111 }));
  });
});
