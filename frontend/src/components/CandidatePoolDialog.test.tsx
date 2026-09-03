import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CandidatePoolDialog from './CandidatePoolDialog';
import { api } from '../api';
import type { CandidatesResponse } from '../types';

const list: CandidatesResponse = {
  side: { club_id: 43, name: 'India', gender: 'male', display_name: 'India (men)' },
  pool: {
    source: 'recency_window',
    window_months: 12,
    since: '2025-09-10',
    size: 2,
    retired_excluded: 1,
  },
  candidates: [
    {
      player_id: 1,
      player_name: 'Current Player',
      is_wicket_keeper: false,
      last_played: '2026-08-01',
      excluded: false,
    },
    {
      player_id: 7,
      player_name: 'MS Dhoni',
      is_wicket_keeper: true,
      last_played: '2019-07-09',
      excluded: true,
      reason: 'retired',
      detail: 'no appearance in any format since 2019-07-09 (5-year bound)',
    },
  ],
};

function renderDialog(overrides: Partial<React.ComponentProps<typeof CandidatePoolDialog>> = {}) {
  const props = {
    open: true,
    onClose: vi.fn(),
    format: 'T20I',
    clubId: 43,
    teamName: 'India (men)',
    matchDate: '2026-09-10',
    allTime: false,
    onAllTimeChange: vi.fn(),
    selected: null,
    onApply: vi.fn(),
    ...overrides,
  };
  render(<CandidatePoolDialog {...props} />);
  return props;
}

describe('CandidatePoolDialog', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(api, 'getCandidates').mockResolvedValue(list);
  });

  it('lists each candidate with the date he last played', async () => {
    renderDialog();

    expect(await screen.findByText('Current Player')).toBeInTheDocument();
    expect(screen.getByText('2026-08-01')).toBeInTheDocument();
    expect(screen.getByText(/played for India \(men\) in the last 12 months/i)).toBeInTheDocument();
  });

  // An exclusion a user cannot see is one they cannot undo, so the excluded player is on
  // the list, struck through, with the reason and an Undo (D-12, step 4).
  it('shows an excluded player with his reason and an undo', async () => {
    const unflag = vi.spyOn(api, 'unflagRetirement').mockResolvedValue({
      player_id: 7,
      flagged: false,
      promoted: false,
      existed: true,
      demoted: true,
    });
    renderDialog();

    const name = await screen.findByText('MS Dhoni (wk)');
    expect(getComputedStyle(name).textDecoration).toContain('line-through');
    await userEvent.click(screen.getByRole('button', { name: /undo/i }));

    await waitFor(() => expect(unflag).toHaveBeenCalledWith(7));
  });

  it('flags a player retired from the list', async () => {
    const flag = vi.spyOn(api, 'flagRetirement').mockResolvedValue({
      player_id: 1,
      flagged: true,
      promoted: false,
      unchecked: ['career_end'],
    });
    renderDialog();

    await screen.findByText('Current Player');
    await userEvent.click(screen.getByRole('button', { name: /retired/i }));

    await waitFor(() => expect(flag).toHaveBeenCalledWith(1, 'T20I'));
  });

  // Manual picking is optional: closing without ticking leaves the default pool.
  it('leaves the default pool in place when the user asks for it', async () => {
    const props = renderDialog();

    await screen.findByText('Current Player');
    await userEvent.click(screen.getByRole('button', { name: /use the default pool/i }));

    expect(props.onApply).toHaveBeenCalledWith(null);
    expect(props.onClose).toHaveBeenCalled();
  });

  it('sends exactly the players the user ticked', async () => {
    const props = renderDialog();

    await screen.findByText('Current Player');
    await userEvent.click(screen.getByRole('checkbox', { name: /pick current player/i }));
    await userEvent.click(screen.getByRole('button', { name: /use these 1 players/i }));

    expect(props.onApply).toHaveBeenCalledWith([1]);
  });

  it('opens on the pick already in force rather than discarding it', async () => {
    renderDialog({ selected: [7] });

    await screen.findByText('MS Dhoni (wk)');
    expect(screen.getByRole('checkbox', { name: /pick ms dhoni/i })).toBeChecked();
  });

  it('asks the backend for the all-time list when widened', async () => {
    renderDialog({ allTime: true });

    await waitFor(() =>
      expect(api.getCandidates).toHaveBeenCalledWith(
        expect.objectContaining({ all_time: true, club_id: 43, format: 'T20I' }),
      ),
    );
  });

  it('loads nothing while it is closed', () => {
    renderDialog({ open: false });

    expect(api.getCandidates).not.toHaveBeenCalled();
  });
});
