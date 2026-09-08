import { describe, it, expect, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import AuctionProjectionPanel from './AuctionProjectionPanel';
import { ApiError } from '../lib/apiError';
import type { AuctionListedPlayer, AuctionProjection } from '../types';

/**
 * The projection surface (P3-2).
 *
 * What is asserted here is the honesty of the rendering rather than its layout: that both
 * intervals appear with their sources named and B-11 and B-14 beside the simulator's, that
 * the three assumptions are on screen as assumptions, that a ground the model has no
 * context for says it reads neutral, that the wicket line carries probabilities and no
 * band, and that a refused projection shows no number at all.
 */

const listed: AuctionListedPlayer[] = [
  { player_id: 2, player_name: 'Bowler Available', state: 'available', state_changed_at: '' },
  { player_id: 3, player_name: 'Batter Available', state: 'available', state_changed_at: '' },
];

function projection(overrides: Partial<AuctionProjection> = {}): AuctionProjection {
  return {
    auction_id: 'auction-1',
    candidate: { player_id: 2, player_name: 'Bowler Available' },
    assumptions: {
      eleven: [
        { player_id: 100, player_name: 'Squad One' },
        { player_id: 2, player_name: 'Bowler Available' },
      ],
      opposition: {
        club_id: 22,
        name: 'Rival Franchise',
        players: [{ player_id: 200, player_name: 'Rival One' }],
      },
      grounds: [
        { venue_id: 4, venue_name: 'Chepauk' },
        { venue_id: 9, venue_name: 'Wankhede' },
      ],
      toss: 'unknown',
      format: 'T20',
      what_a_ground_changes:
        'The performance model reads a ground through two columns only — venue_bf_rate, venue_n. ' +
        'The ground’s scoring level was gated and recorded as a null (A-1) and is not consumed.',
      not_xi_picking:
        'In T20 the system has not shown it can choose an eleven better than rating order.',
    },
    grounds: [
      {
        venue_id: 4,
        venue_name: 'Chepauk',
        ground: { bat_first_rate: 0.47, matches: 83, neutral: false },
        candidate: {
          runs: { q10: 6, median: 24, q90: 51, interval_source: 'l2b_quantiles' },
          balls_faced: { q10: 8, median: 18, q90: 30, interval_source: 'l2b_quantiles' },
          runs_conceded: { q10: 0, median: 12, q90: 34, interval_source: 'l2b_quantiles' },
          wickets: { expected: 0.42, p0: 0.68, p1: 0.24, p2_plus: 0.08, note: 'no q10 or q90' },
          innings_marginalised: true,
        },
        eleven_total: {
          total: { q10: 141, median: 172, q90: 205, interval_source: 'simulator_draws' },
          spread_share: 0.12,
          samples: 2000,
          shared_factor: true,
        },
        toss_marginalised: true,
      },
      {
        venue_id: 9,
        venue_name: 'Wankhede',
        ground: {
          bat_first_rate: 0.5,
          matches: 0,
          neutral: true,
          note: 'The served rating state has no matches at this ground, so it read at the prior.',
        },
        candidate: {
          runs: { q10: 5, median: 22, q90: 48, interval_source: 'l2b_quantiles' },
          balls_faced: { q10: 7, median: 17, q90: 29, interval_source: 'l2b_quantiles' },
          runs_conceded: { q10: 0, median: 11, q90: 33, interval_source: 'l2b_quantiles' },
          wickets: { expected: 0.4, p0: 0.7, p1: 0.23, p2_plus: 0.07, note: 'no q10 or q90' },
          innings_marginalised: true,
        },
        eleven_total: {
          total: { q10: 139, median: 168, q90: 201, interval_source: 'simulator_draws' },
          spread_share: 0.11,
          samples: 2000,
          shared_factor: true,
        },
        toss_marginalised: true,
      },
    ],
    intervals: [
      {
        source: 'l2b_quantiles',
        label: 'L2-B’s quantiles',
        note: 'The performance model’s own 10th and 90th percentile heads for one player.',
      },
      {
        source: 'simulator_draws',
        label: 'The simulator’s draws',
        caveats: ['B-11', 'B-14'],
        note: 'B-11 is open: too narrow by day and too wide at night. B-14 is open.',
      },
    ],
    served_ratings: { run_id: '20260906T083819Z-36689f80', ratings_through: '2026-09-02' },
    ...overrides,
  };
}

function renderPanel(props: Partial<React.ComponentProps<typeof AuctionProjectionPanel>> = {}) {
  const onProject = vi.fn();
  render(
    <AuctionProjectionPanel
      listed={listed}
      projection={projection()}
      projecting={false}
      error={null}
      onProject={onProject}
      {...props}
    />,
  );
  return { onProject };
}

describe('AuctionProjectionPanel', () => {
  it('shows one row per ground with the served medians and their 10-90 bands', () => {
    renderPanel();

    const chepauk = within(screen.getByTestId('auction-ground-4'));
    expect(chepauk.getByText('24.0')).toBeInTheDocument();
    expect(chepauk.getByText('6.0–51.0')).toBeInTheDocument();
    expect(chepauk.getByText('172.0')).toBeInTheDocument();
    expect(chepauk.getByText('141.0–205.0')).toBeInTheDocument();

    const wankhede = within(screen.getByTestId('auction-ground-9'));
    expect(wankhede.getByText('22.0')).toBeInTheDocument();
    expect(wankhede.getByText('168.0')).toBeInTheDocument();
  });

  it('names both interval sources and puts B-11 and B-14 beside the simulator’s', () => {
    renderPanel();

    const quantiles = within(screen.getByTestId('auction-interval-l2b_quantiles'));
    expect(quantiles.getByText(/L2-B’s quantiles/)).toBeInTheDocument();
    expect(quantiles.queryByText('B-11')).not.toBeInTheDocument();

    const draws = within(screen.getByTestId('auction-interval-simulator_draws'));
    expect(draws.getByText(/The simulator’s draws/)).toBeInTheDocument();
    expect(draws.getByText('B-11')).toBeInTheDocument();
    expect(draws.getByText('B-14')).toBeInTheDocument();
  });

  it('shows the eleven, the opposition and the grounds as the assumptions they are', () => {
    renderPanel();

    const conditional = screen.getByTestId('auction-projection-conditional');
    expect(conditional).toHaveTextContent('Bowler Available');
    expect(conditional).toHaveTextContent('Rival Franchise');
    expect(conditional).toHaveTextContent('Chepauk, Wankhede');
    expect(
      within(screen.getByTestId('auction-projection-assumptions')).getByText(/Toss unknown/),
    ).toBeInTheDocument();
  });

  it('says what a ground changes, and that a ground with no context reads neutral', () => {
    renderPanel();

    expect(screen.getByTestId('auction-what-a-ground-changes')).toHaveTextContent('venue_bf_rate');
    expect(screen.getByTestId('auction-what-a-ground-changes')).toHaveTextContent('A-1');
    expect(
      within(screen.getByTestId('auction-ground-9')).getByText('reads neutral'),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId('auction-ground-4')).getByText(/bat-first 0.47 over 83 matches/),
    ).toBeInTheDocument();
  });

  it('shows the wicket line as probabilities and never as a band', () => {
    renderPanel();

    const chepauk = within(screen.getByTestId('auction-ground-4'));
    expect(chepauk.getByText('0.42')).toBeInTheDocument();
    expect(chepauk.getByText(/P\(0\) 0.68/)).toBeInTheDocument();
    expect(chepauk.getByText(/P\(2\+\) 0.08/)).toBeInTheDocument();
  });

  it('shows no win probability and no marginal value anywhere on the surface', () => {
    const { container } = render(
      <AuctionProjectionPanel
        listed={listed}
        projection={projection()}
        projecting={false}
        error={null}
        onProject={vi.fn()}
      />,
    );

    const text = container.textContent ?? '';
    expect(text.toLowerCase()).not.toContain('win probability');
    expect(text.toLowerCase()).not.toContain('marginal value');
    expect(text.toLowerCase()).not.toContain('best xi');
  });

  it('sends the toss the operator set, and omits it while it is unknown', async () => {
    const user = userEvent.setup();
    const { onProject } = renderPanel();

    await user.click(screen.getByLabelText('Candidate'));
    await user.click(await screen.findByRole('option', { name: 'Bowler Available' }));
    await user.click(screen.getByTestId('auction-project'));

    expect(onProject).toHaveBeenCalledWith({ playerId: 2, team1BatsFirst: undefined });

    await user.click(screen.getByLabelText('Toss'));
    await user.click(await screen.findByRole('option', { name: 'His eleven bats first' }));
    await user.click(screen.getByTestId('auction-project'));

    expect(onProject).toHaveBeenLastCalledWith({ playerId: 2, team1BatsFirst: true });
  });

  it('shows a refused projection as a refusal and no number at all', () => {
    renderPanel({
      projection: null,
      error: new ApiError('the likely eleven with this candidate in it holds 10 players', {
        code: 'XI_INCOMPLETE',
        status: 400,
      }),
    });

    expect(screen.getByText(/holds 10 players/)).toBeInTheDocument();
    expect(screen.queryByTestId('auction-ground-rows')).not.toBeInTheDocument();
    expect(screen.queryByTestId('auction-interval-sources')).not.toBeInTheDocument();
  });

  it('shows a mixture only when the operator weighted the grounds', () => {
    renderPanel();
    expect(screen.queryByTestId('auction-mixture')).not.toBeInTheDocument();

    renderPanel({
      projection: projection({
        mixture: {
          total: { q10: 141, median: 172, q90: 201, interval_source: 'simulator_draws' },
          weights: [
            { venue_id: 4, weight: 0.5 },
            { venue_id: 9, weight: 0.5 },
          ],
          note: 'It is not the average of the per-ground ranges.',
        },
      }),
    });
    expect(screen.getByTestId('auction-mixture')).toHaveTextContent('172.0 (141.0–201.0)');
    expect(screen.getByTestId('auction-mixture')).toHaveTextContent('not the average');
  });
});
