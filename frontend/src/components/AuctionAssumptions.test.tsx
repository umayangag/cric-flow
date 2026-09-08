import { describe, it, expect, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import AuctionAssumptions from './AuctionAssumptions';
import type { AuctionListedPlayer, AuctionOppositionSuggestion, AuctionRecord } from '../types';

/**
 * Naming the projection's assumptions (P3-2).
 *
 * The thing worth asserting is that they are named and never assumed: an opposition is
 * seeded from a real side's last recorded eleven, with the date of the match it came from,
 * and the operator edits it before saving. Nothing here assembles a side by rating, and a
 * likely eleven is capped at eleven because a twelfth man is not a place any candidate
 * fills.
 */

const listed: AuctionListedPlayer[] = [
  { player_id: 1, player_name: 'Keeper Sold', state: 'sold', state_changed_at: '' },
  { player_id: 2, player_name: 'Bowler Available', state: 'available', state_changed_at: '' },
  { player_id: 3, player_name: 'Batter Available', state: 'available', state_changed_at: '' },
];

function record(overrides: Partial<AuctionRecord> = {}): AuctionRecord {
  return {
    id: 'auction-1',
    name: 'IPL 2027',
    format: 'T20',
    created_at: '2026-09-08T10:00:00Z',
    buyer: { club_id: 11, name: 'Buying Franchise' },
    venue_ids: [4, 9],
    squad_size: 3,
    min_bowlers: 2,
    require_keeper: true,
    players: listed,
    likely_xi: [{ player_id: 1, player_name: 'Keeper Sold' }],
    ...overrides,
  };
}

const suggestion: AuctionOppositionSuggestion = {
  club_id: 22,
  name: 'Rival Franchise',
  players: Array.from({ length: 11 }, (_, index) => ({
    player_id: 200 + index,
    player_name: `Rival ${index}`,
  })),
  from_match: {
    match_date: '2026-05-24',
    event_name: 'Indian Premier League',
    venue_name: 'Chepauk',
  },
  note: 'It is a starting point to edit, not a prediction of who will play.',
};

function renderAssumptions(props: Partial<React.ComponentProps<typeof AuctionAssumptions>> = {}) {
  const onSave = vi.fn();
  const onSuggest = vi.fn();
  render(
    <AuctionAssumptions
      record={record()}
      listed={listed}
      suggestion={null}
      suggesting={false}
      suggestionError={null}
      onSuggest={onSuggest}
      onSave={onSave}
      saving={false}
      saveError={null}
      {...props}
    />,
  );
  return { onSave, onSuggest };
}

describe('AuctionAssumptions', () => {
  it('starts from the eleven and the opposition the record already holds', () => {
    renderAssumptions({
      record: record({
        opposition: { club_id: 22, name: 'Rival Franchise', players: suggestion.players },
      }),
    });

    expect(
      within(screen.getByTestId('auction-likely-xi')).getByText('Keeper Sold'),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId('auction-opposition')).getByText('Rival 0'),
    ).toBeInTheDocument();
    expect(screen.getByText(/The opposition \(11 of 11\)/)).toBeInTheDocument();
  });

  it('adds a listed player to the likely eleven and takes one back out', async () => {
    const user = userEvent.setup();
    renderAssumptions();

    await user.click(screen.getByLabelText('Add to the likely eleven'));
    await user.click(await screen.findByRole('option', { name: 'Bowler Available' }));

    const eleven = within(screen.getByTestId('auction-likely-xi'));
    expect(eleven.getByText('Bowler Available')).toBeInTheDocument();
    expect(screen.getByText(/The likely eleven \(2 of 11\)/)).toBeInTheDocument();

    await user.click(eleven.getAllByTestId('CancelIcon')[1]);
    expect(screen.getByText(/The likely eleven \(1 of 11\)/)).toBeInTheDocument();
  });

  it('asks for a side’s last eleven and shows the match it came from', async () => {
    const user = userEvent.setup();
    const { onSuggest } = renderAssumptions();

    await user.type(screen.getByLabelText('Club id'), '22');
    await user.click(screen.getByRole('button', { name: 'Use their last eleven' }));

    expect(onSuggest).toHaveBeenCalledWith(22);
  });

  it('takes the suggested eleven as the side being edited, with its date beside it', () => {
    renderAssumptions({ suggestion });

    const banner = screen.getByTestId('auction-opposition-suggestion');
    expect(banner).toHaveTextContent('2026-05-24');
    expect(banner).toHaveTextContent('Indian Premier League');
    expect(banner).toHaveTextContent('starting point');
    expect(
      within(screen.getByTestId('auction-opposition')).getByText('Rival 3'),
    ).toBeInTheDocument();
  });

  it('saves both assumptions, and saves only the eleven while no side is named', async () => {
    const user = userEvent.setup();
    const { onSave } = renderAssumptions();

    await user.click(screen.getByTestId('auction-save-assumptions'));

    expect(onSave).toHaveBeenCalledWith({ likely_xi: [1] });
  });

  it('sends the opposition once one has been named', async () => {
    const user = userEvent.setup();
    const { onSave } = renderAssumptions({ suggestion });

    await user.click(screen.getByTestId('auction-save-assumptions'));

    expect(onSave).toHaveBeenCalledWith({
      likely_xi: [1],
      opposition: { club_id: 22, player_ids: suggestion.players.map((player) => player.player_id) },
    });
  });

  it('says an auction with no grounds cannot be projected at all', () => {
    renderAssumptions({ record: record({ venue_ids: [] }) });

    expect(screen.getByTestId('auction-no-grounds')).toHaveTextContent(
      'a projection is per ground',
    );
  });

  it('shows a refused starting point rather than assembling a side of its own', () => {
    renderAssumptions({
      suggestionError: Object.assign(new Error('no eleven recorded for that side'), {
        name: 'ApiError',
        code: 'NO_FIELDED_ELEVEN',
      }),
    });

    expect(screen.getByText(/no eleven recorded for that side/)).toBeInTheDocument();
    expect(
      within(screen.getByTestId('auction-opposition')).getByText('Nobody named yet.'),
    ).toBeInTheDocument();
  });
});
