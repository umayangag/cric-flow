// @vitest-environment jsdom
import React from 'react';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';
import EvaluateDbTab from '../components/EvaluateDbTab';

// Mock the api module
vi.mock('../api', async () => {
  return {
    api: {
      seasonsNext: vi.fn(),
      listMatches: vi.fn(),
      getMatchSquads: vi.fn(),
      predictWin: vi.fn(),
    },
  };
});

// Types for convenience
type MatchListItem = {
  match_id: number | string;
  date: string;
  teams: [string, string];
  format?: string;
};

const { api } = await import('../api');

describe('EvaluateDbTab', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('happy path: loads matches and evaluates, rendering metrics', async () => {
    // Arrange mocks
    (api.seasonsNext as any).mockResolvedValue({ next_season: 2019 });
    const matches: MatchListItem[] = [
      { match_id: 1, date: '2019-01-10', teams: ['A', 'B'] },
      { match_id: 2, date: '2019-01-11', teams: ['C', 'D'] },
      { match_id: 3, date: '2019-01-12', teams: ['E', 'F'] },
    ];
    (api.listMatches as any).mockResolvedValue(matches);

    // Squads and outcomes
    (api.getMatchSquads as any).mockImplementation(async (id: number) => {
      switch (Number(id)) {
        case 1:
          return {
            match_id: 1,
            date: '2019-01-10',
            teams: ['A', 'B'] as [string, string],
            squads: [
              { team_name: 'A', actual_win: 1, players: [{ player_name: 'p1', runs_scored: 0, balls_faced: 0, fours_scored: 0, sixes_scored: 0, batting_position: 1, strike_rate: 0, runs_conceded: 0, deliveries: 0, wickets_taken: 0, econ: 0 }] },
              { team_name: 'B', actual_win: 0, players: [{ player_name: 'q1', runs_scored: 0, balls_faced: 0, fours_scored: 0, sixes_scored: 0, batting_position: 1, strike_rate: 0, runs_conceded: 0, deliveries: 0, wickets_taken: 0, econ: 0 }] },
            ],
          };
        case 2:
          return {
            match_id: 2,
            date: '2019-01-11',
            teams: ['C', 'D'] as [string, string],
            squads: [
              { team_name: 'C', actual_win: 0, players: [{ player_name: 'p2', runs_scored: 0, balls_faced: 0, fours_scored: 0, sixes_scored: 0, batting_position: 1, strike_rate: 0, runs_conceded: 0, deliveries: 0, wickets_taken: 0, econ: 0 }] },
              { team_name: 'D', actual_win: 1, players: [{ player_name: 'q2', runs_scored: 0, balls_faced: 0, fours_scored: 0, sixes_scored: 0, batting_position: 1, strike_rate: 0, runs_conceded: 0, deliveries: 0, wickets_taken: 0, econ: 0 }] },
            ],
          };
        case 3:
          return {
            match_id: 3,
            date: '2019-01-12',
            teams: ['E', 'F'] as [string, string],
            squads: [
              { team_name: 'E', actual_win: 1, players: [{ player_name: 'p3', runs_scored: 0, balls_faced: 0, fours_scored: 0, sixes_scored: 0, batting_position: 1, strike_rate: 0, runs_conceded: 0, deliveries: 0, wickets_taken: 0, econ: 0 }] },
              { team_name: 'F', actual_win: 0, players: [{ player_name: 'q3', runs_scored: 0, balls_faced: 0, fours_scored: 0, sixes_scored: 0, batting_position: 1, strike_rate: 0, runs_conceded: 0, deliveries: 0, wickets_taken: 0, econ: 0 }] },
            ],
          };
        default:
          throw new Error('unexpected id');
      }
    });

    // Predicts: make one incorrect to avoid 100%
    let predictCall = 0;
    (api.predictWin as any).mockImplementation(async () => {
      predictCall++;
      // For 4th call make it wrong (prob >= 0.5 predicts 1)
      if (predictCall === 4) {
        return { players: [], team_win_probability: 0.9 }; // will be wrong for actual 0
      }
      // Otherwise keep it aligned with actual: first of each pair >= 0.7, second <= 0.3
      const isFirstOfPair = predictCall % 2 === 1;
      return { players: [], team_win_probability: isFirstOfPair ? 0.7 : 0.3 };
    });

    // Render
    render(<EvaluateDbTab />);

    // Set cutoff and format
    const cutoffInput = screen.getByLabelText(/Cutoff date/i) as HTMLInputElement;
    fireEvent.change(cutoffInput, { target: { value: '2018-12-31' } });

    const formatSelect = screen.getByLabelText(/Format/i);
    fireEvent.change(formatSelect, { target: { value: 'T20' } });

    // Load matches
    fireEvent.click(screen.getByRole('button', { name: /Load Matches/i }));

    await screen.findByText(/Loaded 3 matches/i);

    // Evaluate
    fireEvent.click(screen.getByRole('button', { name: /Evaluate/i }));

    // Expect metrics to render
    await screen.findByText(/Teams evaluated:\s*6/i);
    // Accuracy is not exact; just ensure it is shown
    const results = screen.getByText(/Accuracy:/i);
    expect(results).toBeInTheDocument();
  });

  it('shows message when next season is null and does not list matches', async () => {
    (api.seasonsNext as any).mockResolvedValue({ next_season: null });
    const listMatchesSpy = api.listMatches as unknown as ReturnType<typeof vi.fn>;

    render(<EvaluateDbTab />);

    const cutoffInput = screen.getByLabelText(/Cutoff date/i) as HTMLInputElement;
    fireEvent.change(cutoffInput, { target: { value: '2020-12-31' } });

    fireEvent.click(screen.getByRole('button', { name: /Load Matches/i }));

    await screen.findByText(/No next season after cutoff/i);
    expect(listMatchesSpy).not.toHaveBeenCalled();

    // Ensure matches list shows empty state
    const matchesSection = screen.getByText('Matches').closest('section')!;
    expect(within(matchesSection).getByText(/No matches loaded/i)).toBeInTheDocument();
  });

  it('marks match as Error on incomplete squads (422) and continues evaluation', async () => {
    (api.seasonsNext as any).mockResolvedValue({ next_season: 2019 });
    (api.listMatches as any).mockResolvedValue([
      { match_id: 10, date: '2019-02-01', teams: ['G', 'H'] },
      { match_id: 11, date: '2019-02-02', teams: ['I', 'J'] },
    ]);

    (api.getMatchSquads as any).mockImplementation(async (id: number) => {
      if (Number(id) === 10) {
        // Simulate HTTP 422 from API layer
        throw new Error('HTTP 422 Unprocessable Entity: {"code":"INCOMPLETE_SQUADS"}');
      }
      return {
        match_id: 11,
        date: '2019-02-02',
        teams: ['I', 'J'] as [string, string],
        squads: [
          { team_name: 'I', actual_win: 1, players: [{ player_name: 'pi', runs_scored: 0, balls_faced: 0, fours_scored: 0, sixes_scored: 0, batting_position: 1, strike_rate: 0, runs_conceded: 0, deliveries: 0, wickets_taken: 0, econ: 0 }] },
          { team_name: 'J', actual_win: 0, players: [{ player_name: 'pj', runs_scored: 0, balls_faced: 0, fours_scored: 0, sixes_scored: 0, batting_position: 1, strike_rate: 0, runs_conceded: 0, deliveries: 0, wickets_taken: 0, econ: 0 }] },
        ],
      };
    });
    (api.predictWin as any).mockImplementation(async (_players: any, idx: number) => {
      // First of the two teams wins
      return { players: [], team_win_probability: 0.8 };
    });

    render(<EvaluateDbTab />);
    const cutoffInput = screen.getByLabelText(/Cutoff date/i) as HTMLInputElement;
    fireEvent.change(cutoffInput, { target: { value: '2018-12-31' } });
    fireEvent.click(screen.getByRole('button', { name: /Load Matches/i }));
    await screen.findByText(/Loaded 2 matches/i);

    fireEvent.click(screen.getByRole('button', { name: /Evaluate/i }));

    // One match should be Error, the other Evaluated
    const list = await screen.findAllByText(/—/); // list items contain an em status split by em dash
    const html = (await screen.findByText(/Results/)).innerHTML; // ensure results section present
    // Assert statuses directly via list rendering
    const items = screen.getAllByRole('listitem');
    const statuses = items.map((li) => li.textContent || '');
    expect(statuses.some((t) => /Error/i.test(t))).toBe(true);
    expect(statuses.some((t) => /Evaluated/i.test(t))).toBe(true);

    // Teams evaluated should be 2 (only the good match counts both teams)
    await screen.findByText(/Teams evaluated:\s*2/i);
  });

  it('honors concurrency cap of 5 in-flight getMatchSquads calls', async () => {
    (api.seasonsNext as any).mockResolvedValue({ next_season: 2019 });
    const N = 12;
    const matches = Array.from({ length: N }, (_, i) => ({ match_id: 100 + i, date: '2019-03-01', teams: ['X', 'Y'] as [string, string] }));
    (api.listMatches as any).mockResolvedValue(matches);

    let inFlight = 0;
    let maxInFlight = 0;
    (api.getMatchSquads as any).mockImplementation(async () => {
      inFlight++;
      maxInFlight = Math.max(maxInFlight, inFlight);
      // Small delay to simulate network and force concurrency
      await new Promise((res) => setTimeout(res, 10));
      inFlight--;
      return {
        match_id: 0,
        date: '2019-03-01',
        teams: ['X', 'Y'] as [string, string],
        squads: [
          { team_name: 'X', actual_win: 1, players: [{ player_name: 'px', runs_scored: 0, balls_faced: 0, fours_scored: 0, sixes_scored: 0, batting_position: 1, strike_rate: 0, runs_conceded: 0, deliveries: 0, wickets_taken: 0, econ: 0 }] },
          { team_name: 'Y', actual_win: 0, players: [{ player_name: 'py', runs_scored: 0, balls_faced: 0, fours_scored: 0, sixes_scored: 0, batting_position: 1, strike_rate: 0, runs_conceded: 0, deliveries: 0, wickets_taken: 0, econ: 0 }] },
        ],
      };
    });
    (api.predictWin as any).mockResolvedValue({ players: [], team_win_probability: 0.8 });

    render(<EvaluateDbTab />);
    const cutoffInput = screen.getByLabelText(/Cutoff date/i) as HTMLInputElement;
    fireEvent.change(cutoffInput, { target: { value: '2018-12-31' } });
    fireEvent.click(screen.getByRole('button', { name: /Load Matches/i }));
    await screen.findByText(new RegExp(`Loaded ${N} matches`, 'i'));

    const t0 = Date.now();
    fireEvent.click(screen.getByRole('button', { name: /Evaluate/i }));
    await screen.findByText(/Evaluation complete/i, {}, { timeout: 10000 });

    expect(maxInFlight).toBeLessThanOrEqual(5);
    expect((api.getMatchSquads as any).mock.calls.length).toBe(N);
  });
});
