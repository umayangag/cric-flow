import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import UpcomingMatchTab from './UpcomingMatchTab';
import type { PredictTeamSelectionResponse } from '../types';

const mockUseUpcomingMatch = vi.fn();
vi.mock('../hooks/useUpcomingMatch', () => ({
  useUpcomingMatch: () => mockUseUpcomingMatch(),
}));

const baseState = {
  format: 'T20',
  setFormat: vi.fn(),
  team1: 'IND',
  setTeam1: vi.fn(),
  team2: 'AUS',
  setTeam2: vi.fn(),
  venue: '',
  setVenue: vi.fn(),
  matchDate: '2026-09-10',
  setMatchDate: vi.fn(),
  availableFormats: ['T20', 'TEST'],
  availableTeam1s: ['IND'],
  availableTeam2s: ['AUS'],
  venueOptions: [],
  venueLoading: false,
  handleVenueInputChange: vi.fn(),
  loading: false,
  error: null,
  result: null as PredictTeamSelectionResponse | null,
  minDate: '2026-09-01',
  maxDate: '2026-09-15',
  dateError: null,
  canPredict: true,
  handlePredict: vi.fn(),
  maxFutureDays: 14,
  opsStatus: null,
};

function prediction(
  overrides: Partial<PredictTeamSelectionResponse> = {},
): PredictTeamSelectionResponse {
  return {
    team1: [
      {
        player_id: 1,
        player_name: 'Player A',
        runs: 34,
        runs_range: { p10: 8, p90: 71 },
        wickets: 0,
        runs_conceded: 0,
        marginal_value: 0.018,
      },
    ],
    team2: [],
    selection: { objective: 'win', optimised: true },
    win_probability: { team1: 0.61, source: 'display', predicted_winner: 'IND' },
    ...overrides,
  };
}

/**
 * Render the tab with a prediction already in hand.
 *
 * The form is driven by {@link useUpcomingMatch}, whose cascades and date bounds are its
 * own concern; what is asserted here is what the tab does with an answer — which is where
 * the ranges, the source and the not-optimised note have to appear.
 */
function renderWithResult(result: PredictTeamSelectionResponse) {
  mockUseUpcomingMatch.mockReturnValue({ ...baseState, result });
  render(<UpcomingMatchTab />);
}

describe('UpcomingMatchTab', () => {
  beforeEach(() => {
    mockUseUpcomingMatch.mockReset();
    mockUseUpcomingMatch.mockReturnValue(baseState);
  });

  it('renders the form: format, both teams and a date', () => {
    render(<UpcomingMatchTab />);

    expect(screen.getByRole('heading', { name: /upcoming match prediction/i })).toBeInTheDocument();
    expect(screen.getAllByPlaceholderText(/select or type team/i).length).toBe(2);
    expect(screen.getByLabelText(/match date/i)).toBeInTheDocument();
  });

  it('shows the win probability source and the ranges beside the points', () => {
    renderWithResult(prediction());

    expect(screen.getByText(/source: display model/)).toBeInTheDocument();
    expect(screen.getByText('34 (8–71)')).toBeInTheDocument();
    expect(screen.getByText('1.8 pp')).toBeInTheDocument();
    expect(screen.getByText('Best 11 for each team')).toBeInTheDocument();
    expect(screen.queryByText('Not optimised')).not.toBeInTheDocument();
  });

  it('says a rating-ordered XI is not optimised', () => {
    renderWithResult(
      prediction({
        selection: {
          objective: 'ratings',
          optimised: false,
          note: 'Rating-ordered XI: this format has no win objective that ranks.',
        },
      }),
    );

    expect(screen.getByText('Not optimised')).toBeInTheDocument();
    expect(screen.getByText(/no win objective that ranks/)).toBeInTheDocument();
    expect(screen.getByText('Rating-ordered 11 for each team')).toBeInTheDocument();
    expect(screen.queryByText('Marginal')).not.toBeInTheDocument();
  });
});
