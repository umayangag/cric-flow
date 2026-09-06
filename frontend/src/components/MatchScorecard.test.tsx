import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import MatchScorecard from './MatchScorecard';
import type {
  PredictScorecard,
  PredictServedRatings,
  PredictTossSummary,
  PredictWinProbability,
} from '../types';

/** The default toss: unknown, which is the simulator drawing both batting orders. */
const unknownToss: PredictTossSummary = { team1_bats_first: null, honoured: true };

/** The rating state the answer names: the run and the date its ratings run through (P1-5). */
const served: PredictServedRatings = {
  ratings_through: '2026-09-02',
  run_id: '20260906T083819Z-36689f80',
};

const scorecard: PredictScorecard = {
  samples: 2000,
  toss_marginalised: true,
  team1_innings: { total: 171, extras: 9, p10: 130, median: 170, p90: 210 },
  team2_innings: { total: 162, extras: 8, p10: 122, median: 161, p90: 201 },
};

const display: PredictWinProbability = {
  team1: 0.612,
  source: 'display',
  simulated: 0.583,
  predicted_winner: 'IND',
};

describe('MatchScorecard', () => {
  it('names the model the headline probability came from', () => {
    render(
      <MatchScorecard
        scorecard={scorecard}
        winProbability={display}
        toss={unknownToss}
        served={served}
        team1="IND"
        team2="AUS"
      />,
    );

    expect(screen.getByText(/source: display model/)).toBeInTheDocument();
    expect(screen.getByText('61.2%')).toBeInTheDocument();
    expect(screen.getByText(/simulated: 58.3%/)).toBeInTheDocument();
  });

  it('shows each innings total with its 10-90 range and its extras', () => {
    render(
      <MatchScorecard
        scorecard={scorecard}
        winProbability={display}
        toss={unknownToss}
        served={served}
        team1="IND"
        team2="AUS"
      />,
    );

    expect(screen.getByText(/171 runs \(130–210\), extras 9/)).toBeInTheDocument();
    expect(screen.getByText(/162 runs \(122–201\), extras 8/)).toBeInTheDocument();
    // Each innings is named by its side: the response carries team1's innings and team2's,
    // not a first and a second.
    expect(screen.getByText('IND innings:')).toBeInTheDocument();
    expect(screen.getByText('AUS innings:')).toBeInTheDocument();
    expect(screen.getByText('2,000 draws')).toBeInTheDocument();
    expect(screen.getByText('toss unknown: both batting orders averaged')).toBeInTheDocument();
  });

  // Every served prediction carries its date beside the headline, off its own payload
  // (P1-5): a number copied out of the Lab has the date it describes next to it.
  it('shows the date and run the numbers were served from beside the headline', () => {
    render(
      <MatchScorecard
        scorecard={scorecard}
        winProbability={display}
        toss={unknownToss}
        served={served}
        team1="IND"
        team2="AUS"
      />,
    );

    const asOf = screen.getByTestId('ratings-as-of');
    expect(asOf).toHaveTextContent('ratings as of 2026-09-02 · run 20260906T083819Z-36689f80');
    expect(asOf).toHaveAttribute('title', expect.stringMatching(/refused, never served stale/));
  });

  it('says there is no total rather than inventing one', () => {
    render(
      <MatchScorecard
        winProbability={{ team1: 0.44, source: 'display', predicted_winner: 'ENG' }}
        toss={unknownToss}
        served={served}
        team1="AUS"
        team2="ENG"
      />,
    );

    expect(screen.getByText(/no fixed innings length/)).toBeInTheDocument();
    expect(screen.queryByText(/draws/)).not.toBeInTheDocument();
    expect(screen.getByText('ENG')).toBeInTheDocument();
  });

  it('says when the simulator produced the headline', () => {
    render(
      <MatchScorecard
        scorecard={scorecard}
        winProbability={{ ...display, source: 'simulator' }}
        toss={unknownToss}
        served={served}
        team1="IND"
        team2="AUS"
      />,
    );

    expect(screen.getByText(/source: simulator/)).toBeInTheDocument();
  });

  // A known toss has to read as known, and it has to say which side bats first: the
  // response carries the batting order, and the card is where a person reads it (P1-1).
  it('names the side that bats first when the toss is known', () => {
    render(
      <MatchScorecard
        scorecard={{ ...scorecard, toss_marginalised: false }}
        winProbability={display}
        toss={{ team1_bats_first: false, honoured: true }}
        served={served}
        team1="IND"
        team2="AUS"
      />,
    );

    expect(screen.getByText('toss: AUS bats first')).toBeInTheDocument();
    expect(screen.queryByText(/both batting orders averaged/)).not.toBeInTheDocument();
    // Each innings is named by its side and its batting position, because the response
    // carries team1's innings and team2's whichever bats first — "innings 1 (IND)" over a
    // card where AUS bats first would be a label contradicting the numbers beneath it.
    expect(screen.getByText('AUS (batting first):')).toBeInTheDocument();
    expect(screen.getByText('IND (batting second):')).toBeInTheDocument();
  });

  // §8.7: an input the forecast could not use is said so on the answer, not dropped.
  it('says when a named toss could not be used', () => {
    render(
      <MatchScorecard
        winProbability={display}
        toss={{
          team1_bats_first: null,
          honoured: false,
          note: 'This format has no innings length, so the toss you named was not used.',
        }}
        served={served}
        team1="IND"
        team2="AUS"
      />,
    );

    expect(screen.getByText(/the toss you named was not used/)).toBeInTheDocument();
  });
});
