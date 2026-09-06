import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import TeamTable from './TeamTable';
import type { PredictSelectionSummary, PredictTeamSelectedPlayer } from '../types';

const player: PredictTeamSelectedPlayer = {
  player_id: 1,
  player_name: 'Player A',
  runs: 45,
  runs_range: { p10: 12, p90: 88 },
  balls: 32,
  balls_range: { p10: 14, p90: 55 },
  wickets: 2,
  wickets_range: { p10: 0, p90: 4 },
  runs_conceded: 31,
  runs_conceded_range: { p10: 18, p90: 47 },
  economy: 6.2,
  marginal_value: 0.023,
};

const optimised: PredictSelectionSummary = { objective: 'win', optimised: true };
const ratingOrderedSelection: PredictSelectionSummary = {
  objective: 'ratings',
  optimised: false,
  note: 'Rating-ordered XI.',
};

describe('TeamTable', () => {
  it('shows the 10-90 range beside every point', () => {
    render(<TeamTable teamName="India" players={[player]} selection={optimised} />);

    expect(screen.getByText('India')).toBeInTheDocument();
    expect(screen.getByText('Player A')).toBeInTheDocument();
    expect(screen.getByText('45 (12–88)')).toBeInTheDocument();
    expect(screen.getByText('32 (14–55)')).toBeInTheDocument();
    expect(screen.getByText('2.0 (0.0–4.0)')).toBeInTheDocument();
    expect(screen.getByText('31 (18–47)')).toBeInTheDocument();
  });

  it('shows the marginal value beside a player of an optimised XI', () => {
    render(<TeamTable teamName="India" players={[player]} selection={optimised} />);

    expect(screen.getByText('Marginal')).toBeInTheDocument();
    expect(screen.getByText('2.3 pp')).toBeInTheDocument();
  });

  it('hides the marginal column when nothing was maximised', () => {
    const ratingOrdered = { ...player, marginal_value: undefined };
    render(
      <TeamTable teamName="England" players={[ratingOrdered]} selection={ratingOrderedSelection} />,
    );

    expect(screen.queryByText('Marginal')).not.toBeInTheDocument();
    expect(screen.getByText('45 (12–88)')).toBeInTheDocument();
  });

  // A point the wire served with no range says so, rather than reading as a certainty (P1-4).
  it('says a point has no range rather than showing it bare', () => {
    const noRange: PredictTeamSelectedPlayer = {
      player_id: 2,
      player_name: 'Player B',
      runs: 18,
      wickets: 1,
      runs_conceded: 24,
    };
    render(<TeamTable teamName="India" players={[noRange]} selection={ratingOrderedSelection} />);

    expect(screen.getByText('18 (no range)')).toBeInTheDocument();
  });

  // B-8's other half (P1-4): a searched eleven is listed by the marginal value it carries,
  // and every eleven says what its order is.
  it('ranks a searched eleven by marginal value and says so', () => {
    const lower = { ...player, player_id: 2, player_name: 'Player B', marginal_value: 0.041 };
    render(<TeamTable teamName="India" players={[player, lower]} selection={optimised} />);

    const rows = screen.getAllByRole('row').map((row) => row.textContent ?? '');
    expect(rows.findIndex((text) => text.includes('Player B'))).toBeLessThan(
      rows.findIndex((text) => text.includes('Player A')),
    );
    expect(screen.getByTestId('board-order')).toHaveTextContent(/ordered by marginal value/i);
  });

  it('keeps a rating-ordered eleven as served and says which order that is', () => {
    const second = { ...player, player_id: 2, player_name: 'Player B', marginal_value: undefined };
    render(
      <TeamTable
        teamName="England"
        players={[{ ...player, marginal_value: undefined }, second]}
        selection={ratingOrderedSelection}
      />,
    );

    const rows = screen.getAllByRole('row').map((row) => row.textContent ?? '');
    expect(rows.findIndex((text) => text.includes('Player A'))).toBeLessThan(
      rows.findIndex((text) => text.includes('Player B')),
    );
    expect(screen.getByTestId('board-order')).toHaveTextContent(/not the win model's ranking/);
  });

  it('renders an empty table when no players', () => {
    render(<TeamTable teamName="Australia" players={[]} selection={optimised} />);
    expect(screen.getByText('Australia')).toBeInTheDocument();
    expect(screen.getByRole('table')).toBeInTheDocument();
  });

  /**
   * The card is reachable from every selected player, which is the P1-3 gate's first
   * clause. The row's control names the player so a reader (and a screen reader) can tell
   * which eleven's row they are opening.
   */
  it('offers a why-this-player control on every row', () => {
    const second = { ...player, player_id: 2, player_name: 'Player B' };
    render(<TeamTable teamName="India" players={[player, second]} selection={optimised} />);

    expect(screen.getByRole('button', { name: 'Why Player A?' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Why Player B?' })).toBeInTheDocument();
  });
});
