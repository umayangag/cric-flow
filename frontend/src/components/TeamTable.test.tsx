import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import TeamTable from './TeamTable';
import type { PredictTeamSelectedPlayer } from '../types';

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

describe('TeamTable', () => {
  it('shows the 10-90 range beside every point', () => {
    render(<TeamTable teamName="India" players={[player]} optimised />);

    expect(screen.getByText('India')).toBeInTheDocument();
    expect(screen.getByText('Player A')).toBeInTheDocument();
    expect(screen.getByText('45 (12–88)')).toBeInTheDocument();
    expect(screen.getByText('32 (14–55)')).toBeInTheDocument();
    expect(screen.getByText('2.0 (0.0–4.0)')).toBeInTheDocument();
    expect(screen.getByText('31 (18–47)')).toBeInTheDocument();
  });

  it('shows the marginal value beside a player of an optimised XI', () => {
    render(<TeamTable teamName="India" players={[player]} optimised />);

    expect(screen.getByText('Marginal')).toBeInTheDocument();
    expect(screen.getByText('2.3 pp')).toBeInTheDocument();
  });

  it('hides the marginal column when nothing was maximised', () => {
    const ratingOrdered = { ...player, marginal_value: undefined };
    render(<TeamTable teamName="England" players={[ratingOrdered]} optimised={false} />);

    expect(screen.queryByText('Marginal')).not.toBeInTheDocument();
    expect(screen.getByText('45 (12–88)')).toBeInTheDocument();
  });

  it('shows the bare point when a target has no range', () => {
    const noRange: PredictTeamSelectedPlayer = {
      player_id: 2,
      player_name: 'Player B',
      runs: 18,
      wickets: 1,
      runs_conceded: 24,
    };
    render(<TeamTable teamName="India" players={[noRange]} optimised={false} />);

    expect(screen.getByText('18')).toBeInTheDocument();
  });

  it('renders an empty table when no players', () => {
    render(<TeamTable teamName="Australia" players={[]} optimised />);
    expect(screen.getByText('Australia')).toBeInTheDocument();
    expect(screen.getByRole('table')).toBeInTheDocument();
  });
});
