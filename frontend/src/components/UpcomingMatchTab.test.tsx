import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import UpcomingMatchTab from './UpcomingMatchTab';
import type { PredictTeamSelectionResponse, TeamSideOption } from '../types';

const mockUseUpcomingMatch = vi.fn();
vi.mock('../hooks/useUpcomingMatch', () => ({
  useUpcomingMatch: () => mockUseUpcomingMatch(),
}));

const indiaMen: TeamSideOption = {
  club_id: 43,
  name: 'India',
  gender: 'male',
  display_name: 'India (men)',
};
const indiaWomen: TeamSideOption = {
  club_id: 132,
  name: 'India',
  gender: 'female',
  display_name: 'India (women)',
};
const australiaWomen: TeamSideOption = {
  club_id: 12,
  name: 'Australia',
  gender: 'female',
  display_name: 'Australia (women)',
};

const baseState = {
  format: 'T20I',
  setFormat: vi.fn(),
  team1: indiaWomen,
  setTeam1: vi.fn(),
  team2: australiaWomen,
  setTeam2: vi.fn(),
  venue: '',
  setVenue: vi.fn(),
  matchDate: '2026-09-10',
  setMatchDate: vi.fn(),
  availableFormats: ['T20I', 'TEST'],
  availableTeam1s: [indiaMen, indiaWomen],
  availableTeam2s: [australiaWomen],
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
    team1_side: indiaWomen,
    team2_side: australiaWomen,
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
    win_probability: { team1: 0.61, source: 'display', predicted_winner: 'India (women)' },
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

  // The whole of D-10 at the picker: one name, two options, and the user says which.
  it('offers the two sides of one name as distinct options', async () => {
    const user = userEvent.setup();
    render(<UpcomingMatchTab />);

    await user.click(screen.getAllByPlaceholderText(/select or type team/i)[0]);

    const options = await screen.findAllByRole('option');
    expect(options.map((o) => o.textContent)).toEqual(['India (men)', 'India (women)']);
  });

  it('shows the chosen side, not the bare name, in the picker', () => {
    render(<UpcomingMatchTab />);

    const team1 = screen.getAllByPlaceholderText(/select or type team/i)[0] as HTMLInputElement;
    expect(team1.value).toBe('India (women)');
  });

  it('shows the win probability source and the ranges beside the points', () => {
    renderWithResult(prediction());

    expect(screen.getByText(/source: display model/)).toBeInTheDocument();
    expect(screen.getByText('34 (8–71)')).toBeInTheDocument();
    expect(screen.getByText('1.8 pp')).toBeInTheDocument();
    expect(screen.getByText('Best 11 for each team')).toBeInTheDocument();
    expect(screen.queryByText('Not optimised')).not.toBeInTheDocument();
  });

  // The response says which side it scored, and that is what the tables are titled with —
  // not the picker's own state, which is where a mismatch would be invisible (§8.7).
  it('names the sides the response says were scored', () => {
    renderWithResult(prediction());

    // Both the scorecard and the XI table name the side, so each label appears more than once.
    expect(screen.getAllByText('India (women)').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Australia (women)').length).toBeGreaterThan(0);
    expect(screen.queryByText('India (men)')).not.toBeInTheDocument();
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
