import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import PoolSummary from './PoolSummary';
import type { PoolSummary as PoolSummaryDTO } from '../types';

const windowed: PoolSummaryDTO = {
  source: 'recency_window',
  window_months: 12,
  since: '2025-09-10',
  size: 21,
  retired_excluded: 0,
};

describe('PoolSummary', () => {
  // The sentence the plan asks for, verbatim in substance: which team, how long a window,
  // how many players. Before D-12 the pool was all-time and the surface said nothing.
  it('says which team, how long a window, and how many players', () => {
    render(<PoolSummary pool={windowed} teamName="India (men)" />);

    expect(
      screen.getByText(/played for India \(men\) in the last 12 months \(21 players\)/i),
    ).toBeInTheDocument();
  });

  it('says so when the pool was widened to all-time', () => {
    render(
      <PoolSummary
        pool={{ source: 'all_time', size: 96, retired_excluded: 0 }}
        teamName="India (men)"
      />,
    );

    expect(screen.getByText(/everyone who has ever played for India \(men\)/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /all-time pool/i })).not.toBeInTheDocument();
  });

  it('says so when the user chose the pool by hand', () => {
    render(
      <PoolSummary
        pool={{ source: 'manual', size: 14, retired_excluded: 0 }}
        teamName="India (men)"
      />,
    );

    expect(screen.getByText(/14 players you chose for India \(men\)/i)).toBeInTheDocument();
  });

  it('offers the all-time pool one click away from a windowed one', async () => {
    const onWiden = vi.fn();
    render(<PoolSummary pool={windowed} teamName="India (men)" onWiden={onWiden} />);

    await userEvent.click(screen.getByRole('button', { name: /use the all-time pool/i }));

    expect(onWiden).toHaveBeenCalledOnce();
  });

  // §8.7 at the surface: an excluded player is struck through with his reason and an undo,
  // never silently gone.
  it('shows every excluded player, struck through, with the reason and an undo', async () => {
    const onUndo = vi.fn();
    render(
      <PoolSummary
        pool={{
          ...windowed,
          retired_excluded: 1,
          excluded: [
            {
              player_id: 7,
              player_name: 'MS Dhoni',
              last_played: '2019-07-09',
              reason: 'retired',
              detail: 'no appearance in any format since 2019-07-09 (5-year bound)',
            },
          ],
        }}
        teamName="India (men)"
        onUndoExclusion={onUndo}
      />,
    );

    const name = screen.getByText('MS Dhoni');
    expect(name).toBeInTheDocument();
    expect(getComputedStyle(name).textDecoration).toContain('line-through');
    expect(screen.getByText(/1 player excluded as retired/i)).toBeInTheDocument();
    expect(screen.getByText(/last played 2019-07-09/i)).toBeInTheDocument();
    expect(screen.getAllByText(/recorded as retired/i).length).toBeGreaterThan(0);

    await userEvent.click(screen.getByRole('button', { name: /undo/i }));

    expect(onUndo).toHaveBeenCalledWith(7);
  });

  // GO-04 at the surface: a player the other side kept is named with the reason and the
  // evidence. There is no undo, because no flag was set — the way to overrule it is to
  // pick the candidates by hand, which the detail says.
  it('names a candidate the other side kept, with no undo to withdraw', () => {
    const onUndo = vi.fn();
    render(
      <PoolSummary
        pool={{
          ...windowed,
          size: 20,
          excluded: [
            {
              player_id: 9,
              player_name: 'Moved Player',
              last_played: '2026-02-14',
              reason: 'both_sides',
              detail:
                'also a candidate for Chennai Super Kings (men), whom he played for more recently (2026-07-19)',
            },
          ],
        }}
        teamName="Mumbai Indians (men)"
        onUndoExclusion={onUndo}
      />,
    );

    expect(screen.getByText('Moved Player')).toBeInTheDocument();
    expect(screen.getByText(/1 player kept by the other side/i)).toBeInTheDocument();
    expect(screen.getAllByText(/plays for the other side/i).length).toBeGreaterThan(0);
    expect(
      screen.getAllByText(/whom he played for more recently \(2026-07-19\)/i).length,
    ).toBeGreaterThan(0);
    expect(screen.queryByRole('button', { name: /undo/i })).not.toBeInTheDocument();
  });

  // A flag nobody corroborated is one user's opinion, and the surface says exactly that
  // rather than implying a fact about the player.
  it('distinguishes a bare claim from the recorded fact', () => {
    render(
      <PoolSummary
        pool={{
          ...windowed,
          retired_excluded: 1,
          excluded: [{ player_id: 8, player_name: 'A Player', reason: 'user_flagged' }],
        }}
        teamName="India (men)"
      />,
    );

    expect(screen.getAllByText(/nothing has corroborated it/i).length).toBeGreaterThan(0);
  });
});
