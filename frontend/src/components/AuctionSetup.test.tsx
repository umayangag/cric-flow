import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import AuctionSetup, { AUCTION_FORMATS } from './AuctionSetup';
import type { AuctionSummary } from '../types';

const mockTeamSides = vi.fn();
vi.mock('../api', () => ({
  api: { getTeamSidesByFormat: (...args: unknown[]) => mockTeamSides(...args) },
}));

const auctions: AuctionSummary[] = [
  {
    id: 'auction-1',
    name: 'IPL 2027',
    format: 'T20',
    created_at: '2026-09-08T10:00:00Z',
    buyer: { club_id: 11, name: 'Buying Franchise' },
    squad_size: 25,
  },
];

beforeEach(() => {
  vi.clearAllMocks();
  mockTeamSides.mockResolvedValue([
    { club_id: 11, name: 'Buying Franchise', gender: 'male', display_name: 'Buying Franchise' },
  ]);
});

describe('AuctionSetup', () => {
  it('offers only the formats the simulator serves', () => {
    render(
      <AuctionSetup auctions={[]} openAuctionId={null} onOpen={() => {}} onCreate={() => {}} />,
    );

    expect([...AUCTION_FORMATS]).toEqual(['T20', 'T20I', 'ODI']);
    expect(AUCTION_FORMATS).not.toContain('TEST');
  });

  it('loads the sides for the chosen format and names them by their display name', async () => {
    render(
      <AuctionSetup auctions={[]} openAuctionId={null} onOpen={() => {}} onCreate={() => {}} />,
    );

    await waitFor(() => expect(mockTeamSides).toHaveBeenCalledWith('T20'));
    const user = userEvent.setup();
    await user.click(screen.getByLabelText('Buying side'));
    expect(await screen.findByRole('option', { name: 'Buying Franchise' })).toBeInTheDocument();
  });

  it('will not create an auction without a name, a side and a squad to fill', async () => {
    const user = userEvent.setup();
    render(
      <AuctionSetup auctions={[]} openAuctionId={null} onOpen={() => {}} onCreate={() => {}} />,
    );

    expect(screen.getByRole('button', { name: 'Create' })).toBeDisabled();
    await user.type(screen.getByLabelText('Name'), 'IPL 2027');
    expect(screen.getByRole('button', { name: 'Create' })).toBeDisabled();
  });

  it('creates an auction with the eleven’s own constraints', async () => {
    const created: unknown[] = [];
    const user = userEvent.setup();
    render(
      <AuctionSetup
        auctions={[]}
        openAuctionId={null}
        onOpen={() => {}}
        onCreate={(body) => created.push(body)}
      />,
    );
    await waitFor(() => expect(mockTeamSides).toHaveBeenCalled());

    await user.type(screen.getByLabelText('Name'), 'IPL 2027');
    await user.click(screen.getByLabelText('Buying side'));
    await user.click(await screen.findByRole('option', { name: 'Buying Franchise' }));
    await user.click(screen.getByRole('button', { name: 'Create' }));

    expect(created).toEqual([
      {
        name: 'IPL 2027',
        format: 'T20',
        buyer_club_id: 11,
        squad_size: 25,
        min_bowlers: 5,
        require_keeper: true,
      },
    ]);
  });

  it('offers the auctions already on the record, so one survives a reload', async () => {
    const opened: string[] = [];
    const user = userEvent.setup();
    render(
      <AuctionSetup
        auctions={auctions}
        openAuctionId={null}
        onOpen={(id) => opened.push(id)}
        onCreate={() => {}}
      />,
    );

    await user.click(screen.getByLabelText('Open an auction on the record'));
    await user.click(await screen.findByRole('option', { name: /IPL 2027/ }));

    expect(opened).toEqual(['auction-1']);
  });
});
