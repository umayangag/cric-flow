import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import AuctionTab, { NOT_XI_PICKING_SENTENCE } from './AuctionTab';
import { AUCTION_PLAYER_STATES, type AuctionResponse } from '../types';

const mockAuctions = vi.fn();
const mockAuction = vi.fn();
const mockCreateAuction = vi.fn();
const mockAddPlayers = vi.fn();
const mockRecordOutcome = vi.fn();
const mockSearchPlayers = vi.fn();
const mockOpsStatus = vi.fn();
const mockTeamSides = vi.fn();

vi.mock('../api', () => ({
  api: {
    auctions: (...args: unknown[]) => mockAuctions(...args),
    auction: (...args: unknown[]) => mockAuction(...args),
    createAuction: (...args: unknown[]) => mockCreateAuction(...args),
    addAuctionPlayers: (...args: unknown[]) => mockAddPlayers(...args),
    recordAuctionOutcome: (...args: unknown[]) => mockRecordOutcome(...args),
    searchPlayers: (...args: unknown[]) => mockSearchPlayers(...args),
    opsStatus: (...args: unknown[]) => mockOpsStatus(...args),
    getTeamSidesByFormat: (...args: unknown[]) => mockTeamSides(...args),
  },
}));

/** An auction with a keeper sold to the buyer and three players still available. */
function auctionResponse(overrides: Partial<AuctionResponse> = {}): AuctionResponse {
  return {
    auction: {
      id: 'auction-1',
      name: 'IPL 2027',
      format: 'T20',
      created_at: '2026-09-08T10:00:00Z',
      buyer: { club_id: 11, name: 'Buying Franchise' },
      venue_ids: [],
      squad_size: 3,
      min_bowlers: 2,
      require_keeper: true,
      players: [
        {
          player_id: 1,
          player_name: 'Keeper Sold',
          state: 'sold',
          buyer_name: 'Buying Franchise',
          buyer_club_id: 11,
          price: 900,
          state_changed_at: '2026-09-08T11:00:00Z',
          roles: { known: true, roles: ['keeper'] },
        },
        {
          player_id: 2,
          player_name: 'Bowler Available',
          state: 'available',
          state_changed_at: '2026-09-08T10:00:00Z',
          roles: { known: true, roles: ['bowling_option'] },
        },
        {
          player_id: 3,
          player_name: 'Batter Available',
          state: 'available',
          state_changed_at: '2026-09-08T10:00:00Z',
          roles: { known: true, roles: [] },
        },
        {
          player_id: 4,
          player_name: 'Never Seen',
          state: 'available',
          state_changed_at: '2026-09-08T10:00:00Z',
          roles: { known: false, roles: [] },
        },
      ],
    },
    squad: { size: 1, player_ids: [1], players: [] },
    slots: {
      squad_size: 3,
      filled: 1,
      open: 2,
      by_role: {
        keepers: 1,
        bowling_options: 0,
        keeper_needed: false,
        min_bowlers: 2,
        bowling_options_short: 2,
        unknown_roles: 0,
      },
    },
    distribution: { available: 3, keepers: 0, bowling_options: 1, batters: 1, unknown: 1 },
    roles: {
      available: true,
      run_id: '20260906T083819Z-36689f80',
      ratings_through: '2026-09-02',
    },
    ...overrides,
  };
}

const freshOpsStatus = {
  artifacts: { loaded_run: '20260906T083819Z-36689f80' },
  ml: {
    ratings: {
      status: 'fresh',
      ratings_through: '2026-09-02',
      age_days: 2,
      max_age_days: 14,
      fresh: true,
    },
  },
};

beforeEach(() => {
  vi.clearAllMocks();
  mockAuctions.mockResolvedValue({
    auctions: [
      {
        id: 'auction-1',
        name: 'IPL 2027',
        format: 'T20',
        created_at: '2026-09-08T10:00:00Z',
        buyer: { club_id: 11, name: 'Buying Franchise' },
        squad_size: 3,
      },
    ],
  });
  mockAuction.mockResolvedValue(auctionResponse());
  mockOpsStatus.mockResolvedValue(freshOpsStatus);
  mockTeamSides.mockResolvedValue([]);
  mockSearchPlayers.mockResolvedValue({ players: [] });
});

/** Render the tab and open the auction that is on the record. */
async function renderWithOpenAuction() {
  const user = userEvent.setup();
  render(<AuctionTab />);
  await screen.findByLabelText('Open an auction on the record');
  await user.click(screen.getByLabelText('Open an auction on the record'));
  await user.click(await screen.findByRole('option', { name: /IPL 2027/ }));
  await screen.findByTestId('auction-name');
  return user;
}

describe('AuctionTab', () => {
  it('carries the record’s sentence where the numbers are', async () => {
    render(<AuctionTab />);

    expect(await screen.findByTestId('auction-not-xi-picking')).toHaveTextContent(
      /has not shown it can choose an eleven better than rating order/,
    );
    expect(NOT_XI_PICKING_SENTENCE).toMatch(/no "best XI"/);
  });

  it('shows every listed player with the state the record holds for him', async () => {
    await renderWithOpenAuction();

    expect(screen.getByTestId('auction-state-1')).toHaveTextContent('sold');
    expect(screen.getByTestId('auction-state-2')).toHaveTextContent('available');
    expect(within(screen.getByTestId('auction-row-1')).getByText('900')).toBeInTheDocument();
    expect(
      within(screen.getByTestId('auction-row-1')).getByText('Buying Franchise'),
    ).toBeInTheDocument();
  });

  it('labels a known player with neither predicate a batter by elimination, and an unseen one unknown', async () => {
    await renderWithOpenAuction();

    expect(
      within(screen.getByTestId('auction-row-3')).getByText(/batter \(by elimination\)/),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId('auction-row-4')).getByText(/unknown to the served ratings/),
    ).toBeInTheDocument();
  });

  it('renders the remaining pool by role, stamped with the run and the date', async () => {
    await renderWithOpenAuction();

    expect(screen.getByTestId('auction-available-keepers')).toHaveTextContent('keepers: 0');
    expect(screen.getByTestId('auction-available-bowling-options')).toHaveTextContent(
      'bowling options: 1',
    );
    expect(screen.getByTestId('auction-available-batters')).toHaveTextContent('batters: 1');
    expect(screen.getByTestId('auction-available-unknown')).toHaveTextContent(
      'unknown to the served ratings: 1',
    );
    expect(screen.getByTestId('ratings-as-of')).toHaveTextContent('2026-09-02');
    expect(screen.getByTestId('ratings-as-of')).toHaveTextContent('20260906T083819Z-36689f80');
  });

  it('shows the open slots and the constraints they still have to satisfy', async () => {
    await renderWithOpenAuction();

    expect(screen.getByTestId('auction-open-slots')).toHaveTextContent('2 of 3 places open');
    expect(screen.getByTestId('auction-bowling-slot')).toHaveTextContent(
      '2 short of 2 bowling options',
    );
    expect(screen.getByTestId('auction-keeper-slot')).toHaveTextContent('keepers in the squad: 1');
  });

  it('records a sale with its buyer and price, and re-renders from the answer', async () => {
    const afterSale = auctionResponse();
    afterSale.auction.players[1] = {
      ...afterSale.auction.players[1],
      state: 'sold',
      buyer_name: 'Rival Franchise',
      price: 450,
    };
    afterSale.distribution = {
      available: 2,
      keepers: 0,
      bowling_options: 0,
      batters: 1,
      unknown: 1,
    };
    mockRecordOutcome.mockResolvedValue(afterSale);
    const user = await renderWithOpenAuction();

    await user.click(
      within(screen.getByTestId('auction-row-2')).getByRole('button', { name: 'Sold' }),
    );
    await user.type(screen.getByLabelText('Buyer'), 'Rival Franchise');
    await user.type(screen.getByLabelText('Price'), '450');
    await user.click(screen.getByRole('button', { name: 'Record the sale' }));

    await waitFor(() => expect(mockRecordOutcome).toHaveBeenCalled());
    expect(mockRecordOutcome).toHaveBeenCalledWith('auction-1', {
      player_id: 2,
      state: 'sold',
      buyer_name: 'Rival Franchise',
      buyer_club_id: undefined,
      price: 450,
    });
    await waitFor(() =>
      expect(screen.getByTestId('auction-available-bowling-options')).toHaveTextContent(
        'bowling options: 0',
      ),
    );
  });

  it('records an unsold outcome and an undo through the same endpoint', async () => {
    mockRecordOutcome.mockResolvedValue(auctionResponse());
    const user = await renderWithOpenAuction();

    await user.click(
      within(screen.getByTestId('auction-row-2')).getByRole('button', { name: 'Unsold' }),
    );
    await waitFor(() => expect(mockRecordOutcome).toHaveBeenCalled());
    expect(mockRecordOutcome).toHaveBeenLastCalledWith('auction-1', {
      player_id: 2,
      state: 'unsold',
      buyer_name: undefined,
      buyer_club_id: undefined,
      price: undefined,
    });

    await user.click(
      within(screen.getByTestId('auction-row-1')).getByRole('button', { name: 'Undo' }),
    );
    await waitFor(() => expect(mockRecordOutcome).toHaveBeenCalledTimes(2));
    expect(mockRecordOutcome).toHaveBeenLastCalledWith('auction-1', {
      player_id: 1,
      state: 'available',
      buyer_name: undefined,
      buyer_club_id: undefined,
      price: undefined,
    });
  });

  it('on a refused role read shows the list, names the refusal and shows no count off the model', async () => {
    const refused = auctionResponse({
      distribution: undefined,
      roles: {
        available: false,
        code: 'RATINGS_STALE',
        message: 'ratings run through 2026-07-01 (69 days old, limit 14)',
        hint: 'run the retrain step, then reload',
      },
    });
    refused.slots = { squad_size: 3, filled: 1, open: 2 };
    refused.auction.players = refused.auction.players.map((player) => ({
      ...player,
      roles: undefined,
    }));
    mockAuction.mockResolvedValue(refused);
    await renderWithOpenAuction();

    expect(screen.getByTestId('auction-roles-refused')).toHaveTextContent('RATINGS_STALE');
    expect(screen.getByTestId('auction-roles-refused')).toHaveTextContent('69 days old');
    expect(screen.queryByTestId('auction-available-keepers')).not.toBeInTheDocument();
    expect(screen.queryByTestId('auction-keeper-slot')).not.toBeInTheDocument();
    expect(screen.getByTestId('auction-open-slots')).toHaveTextContent('2 of 3 places open');
    expect(screen.getByTestId('auction-row-2')).toHaveTextContent('not read');
    expect(screen.getByTestId('auction-state-2')).toHaveTextContent('available');
  });

  it('offers no column and no control that would be a selection', async () => {
    // The words themselves appear once, in the sentence that says they are absent, so the
    // assertion is about the surface's *columns and controls*: what the tab offers is the
    // record and two role predicates. `opsContract.test.ts` asserts the same rule against
    // the sources, where the endpoints and the payload fields are named.
    await renderWithOpenAuction();

    const headers = screen
      .getAllByRole('columnheader')
      .map((cell) => (cell.textContent ?? '').trim());
    expect(headers).toEqual(['Player', 'State', 'Buyer', 'Price', 'Role', '']);
    const buttons = screen.getAllByRole('button').map((button) => button.textContent ?? '');
    expect(buttons.filter((label) => /optimi|best xi|pick/i.test(label))).toEqual([]);
  });

  it('creates an auction, opens it, and lists players onto it from the search', async () => {
    mockAuctions.mockResolvedValue({ auctions: [] });
    mockTeamSides.mockResolvedValue([
      { club_id: 11, name: 'Buying Franchise', gender: 'male', display_name: 'Buying Franchise' },
    ]);
    mockCreateAuction.mockResolvedValue(auctionResponse());
    mockAddPlayers.mockResolvedValue(auctionResponse());
    mockSearchPlayers.mockResolvedValue({
      players: [
        {
          player_id: 9,
          player_name: 'New Signing',
          is_wicket_keeper: false,
          clubs: [],
          formats: [],
          excluded: false,
        },
      ],
    });
    const user = userEvent.setup();
    render(<AuctionTab />);

    await waitFor(() => expect(mockTeamSides).toHaveBeenCalledWith('T20'));
    await user.type(screen.getByLabelText('Name'), 'IPL 2027');
    await user.click(screen.getByLabelText('Buying side'));
    await user.click(await screen.findByRole('option', { name: 'Buying Franchise' }));
    await user.click(screen.getByRole('button', { name: 'Create' }));

    expect(await screen.findByTestId('auction-name')).toHaveTextContent('IPL 2027');
    expect(mockCreateAuction).toHaveBeenCalledWith({
      name: 'IPL 2027',
      format: 'T20',
      buyer_club_id: 11,
      squad_size: 25,
      min_bowlers: 5,
      require_keeper: true,
    });

    await user.click(screen.getByRole('button', { name: 'Add players' }));
    await user.type(screen.getByLabelText('Player name'), 'new');
    await user.click(screen.getByRole('button', { name: 'Search' }));
    await user.click(await screen.findByTestId('search-result-9'));
    await user.click(screen.getByRole('button', { name: /^Add/ }));

    await waitFor(() => expect(mockAddPlayers).toHaveBeenCalledWith('auction-1', [9]));
  });

  it('says so when no auction is open', async () => {
    mockAuctions.mockResolvedValue({ auctions: [] });
    render(<AuctionTab />);

    expect(await screen.findByText(/No auction is open\. Create one above/)).toBeInTheDocument();
  });

  it('spells the player states the backend does', async () => {
    await renderWithOpenAuction();

    for (const state of AUCTION_PLAYER_STATES) {
      expect(state).toMatch(/^(available|sold|unsold)$/);
    }
    expect(screen.getByTestId('auction-state-1').textContent).toBe('sold');
  });
});
