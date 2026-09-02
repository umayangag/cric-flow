import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import MatchScorecard from './MatchScorecard';
import type { PredictScorecard, PredictWinProbability } from '../types';

const scorecard: PredictScorecard = {
  samples: 2000,
  toss_marginalised: true,
  innings1: { total: 171, extras: 9, p10: 130, median: 170, p90: 210 },
  innings2: { total: 162, extras: 8, p10: 122, median: 161, p90: 201 },
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
      <MatchScorecard scorecard={scorecard} winProbability={display} team1="IND" team2="AUS" />,
    );

    expect(screen.getByText(/source: display model/)).toBeInTheDocument();
    expect(screen.getByText('61.2%')).toBeInTheDocument();
    expect(screen.getByText(/simulated: 58.3%/)).toBeInTheDocument();
  });

  it('shows each innings total with its 10-90 range and its extras', () => {
    render(
      <MatchScorecard scorecard={scorecard} winProbability={display} team1="IND" team2="AUS" />,
    );

    expect(screen.getByText(/171 runs \(130–210\), extras 9/)).toBeInTheDocument();
    expect(screen.getByText(/162 runs \(122–201\), extras 8/)).toBeInTheDocument();
    expect(screen.getByText('2,000 draws')).toBeInTheDocument();
    expect(screen.getByText('toss unknown: both orders averaged')).toBeInTheDocument();
  });

  it('says there is no total rather than inventing one', () => {
    render(
      <MatchScorecard
        winProbability={{ team1: 0.44, source: 'display', predicted_winner: 'ENG' }}
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
        team1="IND"
        team2="AUS"
      />,
    );

    expect(screen.getByText(/source: simulator/)).toBeInTheDocument();
  });
});
