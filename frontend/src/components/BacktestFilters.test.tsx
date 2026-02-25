import React from 'react';
import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { BacktestFilters } from './BacktestFilters';

// Mock API client
vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    ...actual,
    fetchBacktestSelect: vi.fn(),
    fetchFormats: vi.fn(),
    fetchTeamsByFormat: vi.fn(),
    fetchOpponents: vi.fn(),
  };
});

import {
  fetchBacktestSelect,
  fetchFormats,
  fetchTeamsByFormat,
  fetchOpponents,
} from '../api/client';

describe('BacktestFilters component', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (fetchFormats as unknown as Mock).mockResolvedValue(['T20', 'ODI']);
    (fetchTeamsByFormat as unknown as Mock).mockResolvedValue(['IND', 'AUS', 'ENG']);
    (fetchOpponents as unknown as Mock).mockResolvedValue(['AUS', 'ENG']);
  });

  it('renders inputs and search button', async () => {
    const onSelect = vi.fn();
    render(<BacktestFilters onSelect={onSelect} />);
    expect(screen.getByLabelText('format')).toBeInTheDocument();
    expect(screen.getByLabelText('team1')).toBeInTheDocument();
    expect(screen.getByLabelText('team2')).toBeInTheDocument();
    expect(screen.getByLabelText('search')).toBeInTheDocument();
    await waitFor(() => expect(fetchFormats).toHaveBeenCalled());
  });

  it('fetches formats on mount, then teams and opponents in cascade', async () => {
    const onSelect = vi.fn();
    render(<BacktestFilters baseUrl="http://localhost:8080" onSelect={onSelect} />);

    await waitFor(() => expect(fetchFormats).toHaveBeenCalledWith('http://localhost:8080'));
    await waitFor(() =>
      expect(fetchTeamsByFormat).toHaveBeenCalledWith('http://localhost:8080', 'T20'),
    );
    await waitFor(() =>
      expect(fetchOpponents).toHaveBeenCalledWith('http://localhost:8080', 'T20', 'IND'),
    );
  });

  it('Search button is disabled until format, team1, team2 are selected', async () => {
    (fetchFormats as unknown as Mock).mockResolvedValue([]);
    const onSelect = vi.fn();
    render(<BacktestFilters onSelect={onSelect} />);

    await waitFor(() => expect(fetchFormats).toHaveBeenCalled());
    const searchBtn = screen.getByLabelText('search');
    expect(searchBtn).toBeDisabled();
  });

  it('fetches candidates and invokes onSelect on click', async () => {
    const onSelect = vi.fn();
    (fetchBacktestSelect as unknown as Mock).mockResolvedValue({
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

    // Wait for cascade to complete so Search is enabled (formats -> teams -> opponents resolved)
    await waitFor(() => expect(fetchOpponents).toHaveBeenCalled());
    const searchBtn = screen.getByLabelText('search');
    await waitFor(() => expect(searchBtn).not.toBeDisabled());
    fireEvent.click(searchBtn);

    await waitFor(() => expect(fetchBacktestSelect).toHaveBeenCalledTimes(1));
    const table = await screen.findByRole('table', { name: /candidates-table/i });
    expect(table).toBeInTheDocument();
    const selectBtn = await screen.findByLabelText('select-9000111');
    fireEvent.click(selectBtn);
    await waitFor(() => {
      expect(onSelect).toHaveBeenCalledTimes(1);
      expect(onSelect).toHaveBeenCalledWith(9000111, expect.objectContaining({ match_id: 9000111 }));
    });
  });
});
