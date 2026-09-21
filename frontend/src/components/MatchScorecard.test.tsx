import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import MatchScorecard from './MatchScorecard';
import type {
  PredictForecastSummary,
  PredictScorecard,
  PredictSelectionSummary,
  PredictServedRatings,
  PredictTossSummary,
  PredictWinProbability,
} from '../types';

/** The default toss: unknown, which is the simulator drawing both batting orders. */
const unknownToss: PredictTossSummary = { team1_bats_first: null, reading: 'marginalised' };

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

const searched: PredictSelectionSummary = { objective: 'win', optimised: true };
const ratingOrdered: PredictSelectionSummary = {
  objective: 'ratings',
  optimised: false,
  note: 'Rating-ordered XI: no win objective that ranks.',
};
const handBuilt: PredictSelectionSummary = {
  objective: 'fixed',
  optimised: false,
  note: 'Your eleven, scored as picked.',
};

const simulated: PredictForecastSummary = { source: 'simulator' };
const quantiles: PredictForecastSummary = {
  source: 'performance_quantiles',
  note: 'This format has no innings length, so there is no simulated match: the per-player numbers are the performance model’s own quantiles, and there is no total.',
};

type Overrides = Partial<React.ComponentProps<typeof MatchScorecard>>;

function renderCard(overrides: Overrides = {}) {
  return render(
    <MatchScorecard
      scorecard={scorecard}
      winProbability={display}
      forecast={simulated}
      selection={searched}
      toss={unknownToss}
      served={served}
      record={{ stored: true, id: 'f0f8f1a4-0f0e-4a6b-9b6f-2c5d4a1e0004' }}
      team1="IND"
      team2="AUS"
      {...overrides}
    />,
  );
}

describe('MatchScorecard', () => {
  it('names the model the headline probability came from', () => {
    renderCard();

    expect(screen.getByText(/source: display model/)).toBeInTheDocument();
    expect(screen.getByText('61.2%')).toBeInTheDocument();
    expect(screen.getByText(/simulated: 58.3%/)).toBeInTheDocument();
  });

  // P1-4: the model behind the per-player numbers is named by the same rule as the model
  // behind the probability, off the wire's own vocabulary.
  it('names the model the per-player numbers came from', () => {
    renderCard();

    expect(screen.getByText(/per-player numbers: simulated match/)).toBeInTheDocument();
  });

  it('shows each innings total with its 10-90 range and its extras', () => {
    renderCard();

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
    renderCard();

    const asOf = screen.getByTestId('ratings-as-of');
    expect(asOf).toHaveTextContent('ratings as of 2026-09-02 · run 20260906T083819Z-36689f80');
    expect(asOf).toHaveAttribute('title', expect.stringMatching(/refused, never served stale/));
  });

  // Where there is no total, the reason is the response's own sentence, not one this
  // component keeps for the occasion (§8.7, P1-4).
  it('says there is no total in the words the response gave', () => {
    renderCard({
      scorecard: undefined,
      forecast: quantiles,
      winProbability: { team1: 0.44, source: 'display', predicted_winner: 'ENG' },
      team1: 'AUS',
      team2: 'ENG',
    });

    expect(screen.getByTestId('forecast-note')).toHaveTextContent(quantiles.note as string);
    expect(screen.getByText(/per-player numbers: performance quantiles/)).toBeInTheDocument();
    expect(screen.queryByText(/draws/)).not.toBeInTheDocument();
    expect(screen.getByText('ENG')).toBeInTheDocument();
  });

  it('says no reason was served rather than inventing one when the note is absent', () => {
    renderCard({ scorecard: undefined, forecast: { source: 'performance_quantiles' } });

    expect(screen.getByTestId('forecast-note')).toHaveTextContent(/carried no reason/);
  });

  it('says when the simulator produced the headline', () => {
    renderCard({ winProbability: { ...display, source: 'simulator' } });

    expect(screen.getByText(/source: simulator win share/)).toBeInTheDocument();
  });

  // P1-4: beside a probability nothing searched for, the surface says what the number is
  // a read of — and says nothing of the kind where a search did produce it.
  it('labels the probability as a read of a rating-ordered eleven, not a search', () => {
    renderCard({ selection: ratingOrdered });

    expect(screen.getByTestId('win-probability-read-of')).toHaveTextContent(
      "This is the display model's read of this rating-ordered eleven, not the result of a search.",
    );
  });

  it('labels the probability as a read of the eleven the user built', () => {
    renderCard({ selection: handBuilt });

    expect(screen.getByTestId('win-probability-read-of')).toHaveTextContent(
      'read of the eleven you built, not the result of a search',
    );
  });

  it('carries no read-of caveat where the eleven was searched for', () => {
    renderCard();

    expect(screen.queryByTestId('win-probability-read-of')).not.toBeInTheDocument();
  });

  // A known toss has to read as known, and it has to say which side bats first: the
  // response carries the batting order, and the card is where a person reads it (P1-1).
  it('names the side that bats first when the toss is known', () => {
    renderCard({
      scorecard: { ...scorecard, toss_marginalised: false },
      toss: { team1_bats_first: false, reading: 'toss_aware' },
    });

    expect(screen.getByText('toss: AUS bats first')).toBeInTheDocument();
    expect(screen.queryByText(/both batting orders averaged/)).not.toBeInTheDocument();
    // Each innings is named by its side and its batting position, because the response
    // carries team1's innings and team2's whichever bats first — "innings 1 (IND)" over a
    // card where AUS bats first would be a label contradicting the numbers beneath it.
    expect(screen.getByText('AUS (batting first):')).toBeInTheDocument();
    expect(screen.getByText('IND (batting second):')).toBeInTheDocument();
  });

  // §8.7: a toss-aware answer names what in it stayed toss-blind, on the card rather than
  // only in the payload. It used to say the toss "was not used" and blame the format; the
  // toss is used now, and what is carved out is the selection (GO-07).
  it('names what in a toss-aware answer did not read the toss', () => {
    renderCard({
      scorecard: undefined,
      forecast: quantiles,
      toss: {
        team1_bats_first: true,
        reading: 'toss_aware',
        note: 'The eleven was selected on the toss-blind objective.',
      },
    });

    expect(screen.getByText('toss: IND bats first')).toBeInTheDocument();
    expect(screen.getByText(/selected on the toss-blind objective/)).toBeInTheDocument();
  });
});
