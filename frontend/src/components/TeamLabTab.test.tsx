import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import TeamLabTab from './TeamLabTab';
import { api } from '../api';
import { ApiError } from '../lib/apiError';
import type { PredictTeamSelectionResponse, TeamSideOption } from '../types';

const mockUseTeamLab = vi.fn();
vi.mock('../hooks/useTeamLab', () => ({
  useTeamLab: () => mockUseTeamLab(),
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
  team1Pool: { allTime: false, players: null },
  setTeam1Pool: vi.fn(),
  team2Pool: { allTime: false, players: null },
  setTeam2Pool: vi.fn(),
  widenPool: vi.fn(),
  toss: 'unknown' as const,
  setToss: vi.fn(),
  constraints: { minBowlers: '', requireKeeper: true, extraTeam1: '', extraTeam2: '' },
  setConstraints: vi.fn(),
  constraintsError: null,
  play: playState(),
};

/**
 * Play mode's state as the tab receives it (P1-2).
 *
 * The board holds the eleven being built; by default it is the one the answer below it
 * describes, which is every state except a half-finished edit.
 */
function playState(overrides: Partial<ReturnType<typeof defaultPlayState>> = {}) {
  return { ...defaultPlayState(), ...overrides };
}

function defaultPlayState() {
  return {
    team1: [{ player_id: 1, player_name: 'Player A' }],
    team2: [{ player_id: 2, player_name: 'Player B' }],
    teamSize: 1,
    complete: true,
    edited: false,
    addPlayer: vi.fn(),
    removePlayer: vi.fn(),
    swapPlayer: vi.fn(),
    delta: null as ReturnType<typeof import('../lib/playDelta').playDelta>,
    selectedIds: { 1: [1], 2: [2] } as Record<1 | 2, number[]>,
  };
}

/** The default pool a response carries: the measured recency window, nothing excluded. */
const defaultPool = {
  source: 'recency_window' as const,
  window_months: 9,
  since: '2025-12-10',
  size: 24,
  retired_excluded: 0,
};

function prediction(
  overrides: Partial<PredictTeamSelectionResponse> = {},
): PredictTeamSelectionResponse {
  return {
    ratings_through: '2026-09-02',
    run_id: '20260906T083819Z-36689f80',
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
    toss: { team1_bats_first: null, honoured: true },
    team1_pool: defaultPool,
    team2_pool: defaultPool,
    ...overrides,
  };
}

/**
 * Render the Lab with a prediction already in hand.
 *
 * The form is driven by `useTeamLab`, whose cascades and date bounds are its own concern;
 * what is asserted here is what the surface does with an answer — which is where the
 * ranges, the source, the toss and the not-optimised note have to appear.
 */
function renderWithResult(result: PredictTeamSelectionResponse) {
  mockUseTeamLab.mockReturnValue({ ...baseState, result });
  render(<TeamLabTab />);
}

describe('TeamLabTab', () => {
  beforeEach(() => {
    mockUseTeamLab.mockReset();
    mockUseTeamLab.mockReturnValue(baseState);
  });

  it('renders the fixture inputs: format, both teams and a date', () => {
    render(<TeamLabTab />);

    expect(screen.getByRole('heading', { name: /team lab/i })).toBeInTheDocument();
    expect(screen.getAllByPlaceholderText(/select or type team/i).length).toBe(2);
    expect(screen.getByLabelText(/match date/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^optimise$/i })).toBeInTheDocument();
  });

  // The whole of D-10 at the picker: one name, two options, and the user says which.
  it('offers the two sides of one name as distinct options', async () => {
    const user = userEvent.setup();
    render(<TeamLabTab />);

    await user.click(screen.getAllByPlaceholderText(/select or type team/i)[0]);

    const options = await screen.findAllByRole('option');
    expect(options.map((o) => o.textContent)).toEqual(['India (men)', 'India (women)']);
  });

  it('shows the chosen side, not the bare name, in the picker', () => {
    render(<TeamLabTab />);

    const team1 = screen.getAllByPlaceholderText(/select or type team/i)[0] as HTMLInputElement;
    expect(team1.value).toBe('India (women)');
  });

  // The toss is three states and defaults to unknown, which is the marginalised behaviour
  // the stack has always had (P1-1).
  it('offers the three toss states and starts on unknown', async () => {
    const user = userEvent.setup();
    const setToss = vi.fn();
    mockUseTeamLab.mockReturnValue({ ...baseState, setToss });
    render(<TeamLabTab />);

    expect(screen.getByRole('button', { name: /toss unknown/i })).toHaveAttribute(
      'aria-pressed',
      'true',
    );

    await user.click(screen.getByRole('button', { name: /team 2 bats first/i }));

    expect(setToss).toHaveBeenCalledWith('team2_bats_first');
  });

  it('offers the constraints an eleven is picked under', () => {
    render(<TeamLabTab />);

    expect(screen.getByLabelText(/minimum bowlers/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/require a wicketkeeper/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/team 1 must-include player ids/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/team 2 must-include player ids/i)).toBeInTheDocument();
  });

  // Every input the Lab offers goes back to the one hook that builds the request; a control
  // that changed only its own markup would be a second surface with no endpoint behind it.
  it('sends every constraint the user changes back to the hook', async () => {
    const user = userEvent.setup();
    const setConstraints = vi.fn();
    mockUseTeamLab.mockReturnValue({ ...baseState, setConstraints });
    render(<TeamLabTab />);

    await user.type(screen.getByLabelText(/minimum bowlers/i), '5');
    await user.click(screen.getByLabelText(/require a wicketkeeper/i));
    await user.type(screen.getByLabelText(/team 1 must-include player ids/i), '4');
    await user.type(screen.getByLabelText(/team 2 must-include player ids/i), '7');

    expect(setConstraints).toHaveBeenCalledTimes(4);
    // Each call is an update of the constraints already held, so nothing else is dropped.
    const updates = setConstraints.mock.calls.map(([update]) => update(baseState.constraints));
    expect(updates[0]).toEqual({ ...baseState.constraints, minBowlers: '5' });
    expect(updates[1]).toEqual({ ...baseState.constraints, requireKeeper: false });
    expect(updates[2]).toEqual({ ...baseState.constraints, extraTeam1: '4' });
    expect(updates[3]).toEqual({ ...baseState.constraints, extraTeam2: '7' });
  });

  it('sends the fixture the user picks back to the hook', async () => {
    const user = userEvent.setup();
    const setFormat = vi.fn();
    const setTeam2 = vi.fn();
    const setMatchDate = vi.fn();
    const handleVenueInputChange = vi.fn();
    const handlePredict = vi.fn();
    mockUseTeamLab.mockReturnValue({
      ...baseState,
      // Team 2 starts unchosen, so picking it from the list is a change and not a re-pick.
      team2: null,
      setFormat,
      setTeam2,
      setMatchDate,
      handleVenueInputChange,
      handlePredict,
    });
    render(<TeamLabTab />);

    await user.click(screen.getByRole('combobox', { name: /format/i }));
    await user.click(screen.getByRole('option', { name: 'TEST' }));
    expect(setFormat).toHaveBeenCalledWith('TEST');

    await user.click(screen.getAllByPlaceholderText(/select or type team/i)[1]);
    await user.click(await screen.findByRole('option', { name: 'Australia (women)' }));
    expect(setTeam2).toHaveBeenCalledWith(australiaWomen);

    await user.type(screen.getByLabelText(/venue/i), 'Lor');
    expect(handleVenueInputChange).toHaveBeenCalled();

    await user.clear(screen.getByLabelText(/match date/i));
    expect(setMatchDate).toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: /^optimise$/i }));
    expect(handlePredict).toHaveBeenCalled();
  });

  it('refuses to optimise while a constraint cannot be read', () => {
    mockUseTeamLab.mockReturnValue({
      ...baseState,
      canPredict: false,
      constraintsError: 'These team 1 player ids are not ids: abc',
    });
    render(<TeamLabTab />);

    expect(screen.getByText(/these team 1 player ids are not ids: abc/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^optimise$/i })).toBeDisabled();
  });

  it('shows the win probability source and the ranges beside the points', () => {
    renderWithResult(prediction());

    expect(screen.getByText(/source: display model/)).toBeInTheDocument();
    expect(screen.getByText('34 (8–71)')).toBeInTheDocument();
    expect(screen.getByText('1.8 pp')).toBeInTheDocument();
    expect(screen.getByText('Best 11 for each team')).toBeInTheDocument();
    expect(screen.queryByText('Not optimised')).not.toBeInTheDocument();
  });

  // P1-5: the date and run beside the headline come from the prediction's own payload, so
  // they describe the run that answered and not whatever a status poll sees loaded now.
  it('shows the date and run every served prediction carries', () => {
    renderWithResult(prediction());

    expect(screen.getByTestId('ratings-as-of')).toHaveTextContent(
      'ratings as of 2026-09-02 · run 20260906T083819Z-36689f80',
    );
  });

  // H-11 on the surface: a stale registry's refusal is shown with the date, the age against
  // the limit and the step that fixes it -- and no number, because none was served.
  it('shows a RATINGS_STALE refusal with its date and no stale number', () => {
    mockUseTeamLab.mockReturnValue({
      ...baseState,
      result: null,
      error: new ApiError('ratings run through 2026-08-20 (17 days old, limit 14)', {
        status: 503,
        code: 'RATINGS_STALE',
        hint: 'run the retrain step, then reload -- or raise XI_RATINGS_MAX_AGE_DAYS if this is deliberate',
      }),
    });
    render(<TeamLabTab />);

    const refusal = screen.getByRole('alert');
    expect(refusal).toHaveTextContent('Prediction failed');
    expect(refusal).toHaveTextContent('2026-08-20 (17 days old, limit 14)');
    expect(refusal).toHaveTextContent('run the retrain step, then reload');
    expect(refusal).toHaveTextContent('Ops → Pipeline: run Retrain, then Reload');
    expect(refusal).toHaveTextContent('RATINGS_STALE');
    expect(screen.queryByText(/win probability/i)).not.toBeInTheDocument();
    expect(screen.queryByTestId('ratings-as-of')).not.toBeInTheDocument();
  });

  // The surface says which toss the numbers assume, read off the response rather than off
  // the control the user last touched.
  it('names the toss the answer assumed', () => {
    renderWithResult(prediction({ toss: { team1_bats_first: true, honoured: true } }));

    expect(screen.getByText('toss: India (women) bats first')).toBeInTheDocument();
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

  // D-12 at the surface: the answer says which candidates it was chosen out of, and the
  // all-time pool is one click away — which predicts again rather than leaving a number
  // the new pool did not produce beside a line describing it.
  it('says which pool each XI came from, and offers the all-time one', async () => {
    const user = userEvent.setup();
    const widenPool = vi.fn();
    mockUseTeamLab.mockReturnValue({ ...baseState, widenPool, result: prediction() });
    render(<TeamLabTab />);

    expect(
      screen.getAllByText(/played for India \(women\) in the last 9 months \(24 players\)/i).length,
    ).toBeGreaterThan(0);

    await user.click(screen.getAllByRole('button', { name: /use the all-time pool/i })[0]);

    expect(widenPool).toHaveBeenCalledWith(1);
  });

  // Nothing about a pool is silent: an exclusion is named, struck through and reversible.
  it('names every player the ledger excluded from a pool', () => {
    renderWithResult(
      prediction({
        team1_pool: {
          ...defaultPool,
          size: 23,
          retired_excluded: 1,
          excluded: [
            {
              player_id: 77,
              player_name: 'Retired Player',
              last_played: '2015-03-29',
              reason: 'retired',
              detail: 'no match in any format since 2015-03-29',
            },
          ],
        },
      }),
    );

    expect(screen.getByText(/1 player excluded as retired/i)).toBeInTheDocument();
    expect(screen.getByText('Retired Player')).toBeInTheDocument();
    expect(screen.getByText(/no match in any format since 2015-03-29/)).toBeInTheDocument();
    expect(screen.getByText(/last played 2015-03-29/)).toBeInTheDocument();
  });

  // Manual picking is optional and off: the button is there, and opening it is a decision.
  it('opens the candidate list for one side', async () => {
    const user = userEvent.setup();
    vi.spyOn(api, 'getCandidates').mockResolvedValue({
      side: indiaWomen,
      pool: {
        source: 'recency_window',
        window_months: 9,
        since: '2025-12-10',
        size: 0,
        retired_excluded: 0,
      },
      candidates: [],
    });
    render(<TeamLabTab />);

    await user.click(screen.getByRole('button', { name: /choose team 1 candidates/i }));

    expect(await screen.findByText(/candidates for India \(women\)/i)).toBeInTheDocument();
    expect(api.getCandidates).toHaveBeenCalledWith(
      expect.objectContaining({ format: 'T20I', club_id: 132 }),
    );
  });

  it('offers the all-time pool for the second side too', async () => {
    const user = userEvent.setup();
    const widenPool = vi.fn();
    mockUseTeamLab.mockReturnValue({ ...baseState, widenPool, result: prediction() });
    render(<TeamLabTab />);

    await user.click(screen.getAllByRole('button', { name: /use the all-time pool/i })[1]);

    expect(widenPool).toHaveBeenCalledWith(2);
  });

  // A pool chosen by hand is a decision about the prediction: it survives the dialog
  // closing, so the dialog hands it back to the side it was chosen for (D-12).
  it('keeps a hand-picked pool when the candidate list closes', async () => {
    const user = userEvent.setup();
    const setTeam1Pool = vi.fn();
    vi.spyOn(api, 'getCandidates').mockResolvedValue({
      side: indiaWomen,
      pool: {
        source: 'recency_window',
        window_months: 9,
        since: '2025-12-10',
        size: 1,
        retired_excluded: 0,
      },
      candidates: [
        { player_id: 8, player_name: 'Pickable Player', is_wicket_keeper: false, excluded: false },
      ],
    });
    mockUseTeamLab.mockReturnValue({ ...baseState, setTeam1Pool });
    render(<TeamLabTab />);
    await user.click(screen.getByRole('button', { name: /choose team 1 candidates/i }));
    await screen.findByText(/candidates for India \(women\)/i);

    await user.click(screen.getByLabelText(/show everyone who has ever played/i));
    expect(setTeam1Pool.mock.calls[0][0](baseState.team1Pool)).toEqual({
      allTime: true,
      players: null,
    });

    await user.click(screen.getByRole('checkbox', { name: /pick pickable player/i }));
    await user.click(screen.getByRole('button', { name: /use these 1 players/i }));
    expect(setTeam1Pool.mock.calls[1][0](baseState.team1Pool)).toEqual({
      allTime: false,
      players: [8],
    });
    expect(screen.queryByText(/candidates for India \(women\)/i)).not.toBeInTheDocument();
  });

  it('says how many players a hand-picked pool holds', () => {
    mockUseTeamLab.mockReturnValue({
      ...baseState,
      team1Pool: { allTime: false, players: [1, 2, 3] },
    });
    render(<TeamLabTab />);

    expect(screen.getByRole('button', { name: /team 1 pool: 3 chosen/i })).toBeInTheDocument();
  });

  // Play mode (P1-2): the board is the eleven being built, every change re-scores, and a
  // constraint the eleven breaks is on the surface rather than repaired underneath it.
  describe('play mode', () => {
    const constraints = {
      team_size: 1,
      min_bowlers: 5,
      require_keeper: true,
      team1: { size: 1, bowlers: 4, has_keeper: false, met: false },
      team2: { size: 1, bowlers: 6, has_keeper: true, met: true },
    };

    function renderPlay(play: Partial<ReturnType<typeof defaultPlayState>> = {}, result = {}) {
      mockUseTeamLab.mockReturnValue({
        ...baseState,
        play: playState(play),
        result: prediction({ constraints, ...result }),
      });
      render(<TeamLabTab />);
    }

    it('lists both elevens with a swap and a remove on every player', () => {
      renderPlay();

      expect(screen.getByRole('heading', { name: /play mode/i })).toBeInTheDocument();
      expect(screen.getAllByRole('button', { name: /^swap$/i })).toHaveLength(2);
      expect(screen.getByRole('button', { name: /remove player a/i })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /optimise again/i })).toBeInTheDocument();
    });

    it('removes a player through the hook rather than in the markup', async () => {
      const user = userEvent.setup();
      const removePlayer = vi.fn();
      renderPlay({ removePlayer });

      await user.click(screen.getByRole('button', { name: /remove player a/i }));

      expect(removePlayer).toHaveBeenCalledWith(1, 1);
    });

    it('shows a broken constraint as broken, with the counts behind it', () => {
      renderPlay();

      expect(screen.getByText(/bowlers 4 of 5 — broken/i)).toBeInTheDocument();
      expect(screen.getByText(/no keeper — broken/i)).toBeInTheDocument();
      expect(screen.getAllByText(/scored as you built it/i).length).toBeGreaterThan(0);
    });

    it('says an unfinished eleven is not the eleven the numbers describe', () => {
      renderPlay({ team1: [], complete: false });

      expect(screen.getByText(/this eleven is not scored yet/i)).toBeInTheDocument();
      expect(screen.getByText(/0 of 1/)).toBeInTheDocument();
    });

    it('shows what the last change did to the probability and the totals', () => {
      renderPlay({
        delta: {
          winProbability: { previous: 0.61, current: 0.643, change: 0.033 },
          team1Innings: {
            total: { previous: 176, current: 182, change: 6 },
            p10: { previous: 140, current: 145, change: 5 },
            p90: { previous: 212, current: 219, change: 7 },
          },
          team2Innings: null,
        },
      });

      expect(screen.getByLabelText(/change from the previous eleven/i)).toBeInTheDocument();
      expect(screen.getByText('+3.3 pp')).toBeInTheDocument();
      expect(screen.getByText(/\+6\.0 runs \(\+5\.0 \/ \+7\.0\)/)).toBeInTheDocument();
    });

    it('swaps a player for one picked out of the candidate list', async () => {
      const user = userEvent.setup();
      const swapPlayer = vi.fn();
      vi.mocked(api.getCandidates).mockResolvedValue({
        side: indiaWomen,
        pool: {
          source: 'all_time',
          size: 1,
          retired_excluded: 0,
        },
        candidates: [
          {
            player_id: 8,
            player_name: 'Pickable Player',
            is_wicket_keeper: false,
            excluded: false,
          },
        ],
      });
      renderPlay({ swapPlayer });

      await user.click(screen.getAllByRole('button', { name: /^swap$/i })[0]);
      await screen.findByText(/replace player a/i);
      await user.click(await screen.findByText(/pickable player/i));

      expect(swapPlayer).toHaveBeenCalledWith(1, 1, {
        player_id: 8,
        player_name: 'Pickable Player',
      });
    });
  });
});
