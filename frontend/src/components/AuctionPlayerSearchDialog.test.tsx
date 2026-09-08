import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import AuctionPlayerSearchDialog from './AuctionPlayerSearchDialog';
import type { PlayerSearchResult } from '../types';

const mockSearchPlayers = vi.fn();
vi.mock('../api', () => ({
  api: { searchPlayers: (...args: unknown[]) => mockSearchPlayers(...args) },
}));

function result(overrides: Partial<PlayerSearchResult> = {}): PlayerSearchResult {
  return {
    player_id: 7,
    player_name: 'V Kohli',
    is_wicket_keeper: false,
    last_played: '2026-05-30',
    clubs: ['Royal Challengers Bengaluru', 'India'],
    formats: ['T20', 'ODI'],
    excluded: false,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mockSearchPlayers.mockResolvedValue({ players: [result()] });
});

describe('AuctionPlayerSearchDialog', () => {
  it('searches across clubs and shows the clubs and formats a player has played', async () => {
    const user = userEvent.setup();
    render(
      <AuctionPlayerSearchDialog
        open
        onClose={() => {}}
        format="T20"
        listedIds={[]}
        onAdd={() => {}}
      />,
    );

    await user.type(screen.getByLabelText('Player name'), 'kohli');
    await user.click(screen.getByRole('button', { name: 'Search' }));

    await waitFor(() =>
      expect(mockSearchPlayers).toHaveBeenCalledWith({ q: 'kohli', format: 'T20' }),
    );
    expect(await screen.findByText('V Kohli')).toBeInTheDocument();
    expect(screen.getByText(/Royal Challengers Bengaluru, India/)).toBeInTheDocument();
    expect(screen.getByText(/T20, ODI/)).toBeInTheDocument();
    expect(screen.getByText(/last played 2026-05-30/)).toBeInTheDocument();
  });

  it('refuses to search on a prefix too short to mean anything', async () => {
    const user = userEvent.setup();
    render(
      <AuctionPlayerSearchDialog
        open
        onClose={() => {}}
        format="T20"
        listedIds={[]}
        onAdd={() => {}}
      />,
    );

    await user.type(screen.getByLabelText('Player name'), 'k');

    expect(screen.getByRole('button', { name: 'Search' })).toBeDisabled();
    expect(mockSearchPlayers).not.toHaveBeenCalled();
  });

  it('names the database flag as the database’s, not as the model’s role', async () => {
    mockSearchPlayers.mockResolvedValue({
      players: [result({ player_id: 9, player_name: 'MS Dhoni', is_wicket_keeper: true })],
    });
    const user = userEvent.setup();
    render(
      <AuctionPlayerSearchDialog
        open
        onClose={() => {}}
        format="T20"
        listedIds={[]}
        onAdd={() => {}}
      />,
    );

    await user.type(screen.getByLabelText('Player name'), 'dhoni');
    await user.click(screen.getByRole('button', { name: 'Search' }));

    expect(await screen.findByText('database flag: wicket-keeper')).toBeInTheDocument();
  });

  it('shows the ledger’s verdict beside a name rather than instead of one', async () => {
    mockSearchPlayers.mockResolvedValue({
      players: [result({ excluded: true, reason: 'retired' })],
    });
    const user = userEvent.setup();
    render(
      <AuctionPlayerSearchDialog
        open
        onClose={() => {}}
        format="T20"
        listedIds={[]}
        onAdd={() => {}}
      />,
    );

    await user.type(screen.getByLabelText('Player name'), 'kohli');
    await user.click(screen.getByRole('button', { name: 'Search' }));

    expect(await screen.findByText('V Kohli')).toBeInTheDocument();
    expect(screen.getByText('ledger: retired')).toBeInTheDocument();
  });

  it('adds the players that were ticked and offers nobody already on the list twice', async () => {
    mockSearchPlayers.mockResolvedValue({
      players: [result(), result({ player_id: 8, player_name: 'R Sharma' })],
    });
    const added: number[][] = [];
    const user = userEvent.setup();
    render(
      <AuctionPlayerSearchDialog
        open
        onClose={() => {}}
        format="T20"
        listedIds={[7]}
        onAdd={(ids) => added.push(ids)}
      />,
    );

    await user.type(screen.getByLabelText('Player name'), 'a');
    await user.type(screen.getByLabelText('Player name'), 'r{Enter}');
    expect(await screen.findByText('already listed')).toBeInTheDocument();
    expect(screen.getByTestId('search-result-7')).toHaveClass('Mui-disabled');

    await user.click(screen.getByTestId('search-result-8'));
    await user.click(screen.getByRole('button', { name: /^Add/ }));

    expect(added).toEqual([[8]]);
  });

  it('says so when no player of that name is in the database', async () => {
    mockSearchPlayers.mockResolvedValue({ players: [] });
    const user = userEvent.setup();
    render(
      <AuctionPlayerSearchDialog
        open
        onClose={() => {}}
        format="T20"
        listedIds={[]}
        onAdd={() => {}}
      />,
    );

    await user.type(screen.getByLabelText('Player name'), 'zzz');
    await user.click(screen.getByRole('button', { name: 'Search' }));

    expect(
      await screen.findByText('No player of that name is in this database.'),
    ).toBeInTheDocument();
  });
});
