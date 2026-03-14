import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import TeamTable from './TeamTable';

describe('TeamTable', () => {
  it('renders team name and player stats', () => {
    const players = [
      {
        player_id: 1,
        player_name: 'Player A',
        runs: 45.5,
        wickets: 2,
        economy: 6.2,
        catches: 1,
        run_outs: 0,
      },
      {
        player_id: 2,
        player_name: 'Player B',
        runs: 12.0,
        wickets: 0,
        economy: 8.5,
        catches: 0,
        run_outs: 1,
      },
    ];
    render(<TeamTable teamName="India" players={players} />);
    expect(screen.getByText('India')).toBeInTheDocument();
    expect(screen.getByText('Player A')).toBeInTheDocument();
    expect(screen.getByText('Player B')).toBeInTheDocument();
    expect(screen.getByText('45.5')).toBeInTheDocument();
    expect(screen.getByText('2.0')).toBeInTheDocument();
    expect(screen.getByText('6.20')).toBeInTheDocument();
    expect(screen.getByText('12.0')).toBeInTheDocument();
    expect(screen.getByText('0.0')).toBeInTheDocument();
    expect(screen.getByText('8.50')).toBeInTheDocument();
    expect(screen.getByRole('table')).toBeInTheDocument();
    expect(screen.getAllByText('1')).toHaveLength(2);
    expect(screen.getAllByText('0')).toHaveLength(2);
  });

  it('renders empty table when no players', () => {
    render(<TeamTable teamName="Australia" players={[]} />);
    expect(screen.getByText('Australia')).toBeInTheDocument();
    expect(screen.getByRole('table')).toBeInTheDocument();
  });
});
