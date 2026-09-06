import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import WhyThisPlayer from './WhyThisPlayer';
import type {
  PredictSelectionReason,
  PredictSelectionSummary,
  PredictTeamSelectedPlayer,
} from '../types';

const reason: PredictSelectionReason = {
  roles: ['keeper', 'bowling_option'],
  selection_rating: 1.37,
  rating_percentile: 92.3,
  pool_size: 27,
  best_alternative: { player_id: 9, player_name: 'Player Z', win_probability_gap: 0.014 },
};

const player: PredictTeamSelectedPlayer = {
  player_id: 1,
  player_name: 'Player A',
  runs: 45,
  runs_range: { p10: 12, p90: 88 },
  wickets: 2,
  wickets_range: { p10: 0, p90: 4 },
  runs_conceded: 31,
  marginal_value: 0.023,
  selection_reason: reason,
};

const optimised: PredictSelectionSummary = { objective: 'win', optimised: true };
const ratingOrdered: PredictSelectionSummary = {
  objective: 'ratings',
  optimised: false,
  note: 'Rating-ordered XI.',
};
const handBuilt: PredictSelectionSummary = { objective: 'fixed', optimised: false };

describe('WhyThisPlayer', () => {
  describe('the searched eleven', () => {
    it('leads with the marginal value and names the best alternative left in the pool', () => {
      render(<WhyThisPlayer player={player} selection={optimised} />);

      expect(screen.getByText('Why Player A?')).toBeInTheDocument();
      expect(screen.getByText(/2\.3 pp of win probability/)).toBeInTheDocument();
      expect(screen.getByText(/1\.4 pp better than swapping in Player Z/)).toBeInTheDocument();
    });

    /**
     * The gap comes out of one evaluation of the objective per candidate swap, which
     * yields no interval. § 4's rule is to say so rather than invent one.
     */
    it('says the gap is a point estimate rather than showing an interval it does not have', () => {
      render(<WhyThisPlayer player={player} selection={optimised} />);

      expect(
        screen.getByText(/point estimate here; this computation yields no interval/),
      ).toBeInTheDocument();
    });

    it('shows the roles the constraints counted and the pool the percentile is over', () => {
      render(<WhyThisPlayer player={player} selection={optimised} />);

      expect(screen.getByText('Keeper')).toBeInTheDocument();
      expect(screen.getByText('Bowling option')).toBeInTheDocument();
      expect(screen.getByText(/Ahead of 92 % of the 27 candidates/)).toBeInTheDocument();
    });

    it('shows the expected contribution with the band the model gave it', () => {
      render(<WhyThisPlayer player={player} selection={optimised} />);

      expect(screen.getByText(/45 \(12–88\) runs/)).toBeInTheDocument();
      expect(screen.getByText(/2\.0 \(0\.0–4\.0\) wickets/)).toBeInTheDocument();
    });

    it('shows why the objective could name no alternative instead of dropping the line', () => {
      const noAlternative: PredictTeamSelectedPlayer = {
        ...player,
        selection_reason: {
          ...reason,
          best_alternative: undefined,
          best_alternative_note: 'no swap keeps the eleven inside its constraints',
        },
      };
      render(<WhyThisPlayer player={noAlternative} selection={optimised} />);

      expect(
        screen.getByText('no swap keeps the eleven inside its constraints'),
      ).toBeInTheDocument();
    });

    /**
     * A marginal value can be negative — the objective can prefer an average player to a
     * selected one — and the card says what that means rather than printing a bare minus.
     */
    it('reads a negative marginal value out loud instead of leaving a bare minus', () => {
      const negative = { ...player, marginal_value: -0.0238 };
      render(<WhyThisPlayer player={negative} selection={optimised} />);

      expect(screen.getByText(/-2\.4 pp of win probability/)).toBeInTheDocument();
      expect(
        screen.getByText(/scores the eleven higher with that average player/),
      ).toBeInTheDocument();
    });

    it('states plainly that a player answered neither constraint', () => {
      const plain: PredictTeamSelectedPlayer = {
        ...player,
        selection_reason: { ...reason, roles: [] },
      };
      render(<WhyThisPlayer player={plain} selection={optimised} />);

      expect(screen.getByText(/picked on his rating alone/)).toBeInTheDocument();
    });
  });

  describe('the rating-ordered eleven', () => {
    const ratingOrderedPlayer: PredictTeamSelectedPlayer = {
      ...player,
      marginal_value: undefined,
      selection_reason: { ...reason, best_alternative: undefined },
    };

    it('says the simpler truth and shows the rating with its pool percentile', () => {
      render(<WhyThisPlayer player={ratingOrderedPlayer} selection={ratingOrdered} />);

      expect(screen.getByText('Picked by rating, not by the win model.')).toBeInTheDocument();
      expect(screen.getByText('1.37')).toBeInTheDocument();
      expect(screen.getByText(/Ahead of 92 % of the 27 candidates/)).toBeInTheDocument();
    });

    /** The P1-3 gate: nothing the win model would have said appears on this card. */
    it('carries no marginal value and no beats-whom line', () => {
      render(<WhyThisPlayer player={ratingOrderedPlayer} selection={ratingOrdered} />);

      expect(screen.queryByText(/of win probability/)).not.toBeInTheDocument();
      expect(screen.queryByText(/better than swapping in/)).not.toBeInTheDocument();
      expect(screen.queryByText('Ahead of the next best')).not.toBeInTheDocument();
    });

    /**
     * B-8's honest half: the ordering on screen is not the display model's ranking, and
     * the card is where a user learns it before acting on the order.
     */
    it('warns that the ordering it shows is not the win model’s ranking', () => {
      render(<WhyThisPlayer player={ratingOrderedPlayer} selection={ratingOrdered} />);

      expect(screen.getByText(/not the win model's ranking/)).toBeInTheDocument();
    });

    it('still shows the expected contribution with its range', () => {
      render(<WhyThisPlayer player={ratingOrderedPlayer} selection={ratingOrdered} />);

      expect(screen.getByText(/45 \(12–88\) runs/)).toBeInTheDocument();
    });

    /**
     * Even handed a marginal value it must not print one: the format's policy scoped the
     * win objective off, so there is no marginal value to show whatever the payload says.
     */
    it('prints no marginal value even if one reached it', () => {
      const stray = { ...ratingOrderedPlayer, marginal_value: 0.031 };
      render(<WhyThisPlayer player={stray} selection={ratingOrdered} />);

      expect(screen.queryByText(/3\.1 pp/)).not.toBeInTheDocument();
    });
  });

  describe('a hand-built eleven', () => {
    it('says nothing selected him and shows only what the models still say', () => {
      const pinned: PredictTeamSelectedPlayer = {
        ...player,
        marginal_value: undefined,
        selection_reason: undefined,
      };
      render(<WhyThisPlayer player={pinned} selection={handBuilt} />);

      expect(screen.getByText(/You built this eleven/)).toBeInTheDocument();
      expect(screen.getByText(/45 \(12–88\) runs/)).toBeInTheDocument();
      expect(screen.queryByText('Selection rating')).not.toBeInTheDocument();
    });
  });
});
