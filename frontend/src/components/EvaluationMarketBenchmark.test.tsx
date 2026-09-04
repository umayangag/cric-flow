import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import EvaluationMarketBenchmark from './EvaluationMarketBenchmark';
import type { MarketBenchmark, MarketBenchmarkFormat } from '../types';

function benchmark(formats: Record<string, MarketBenchmarkFormat>): MarketBenchmark {
  return {
    available: true,
    source: {
      name: 'Betfair Exchange Match Odds season summaries (BBL, WBBL)',
      url: 'https://example.invalid/odds',
      licence: 'cached locally and never committed',
      cached_dir: '../data/market-odds',
      priced_at: 'best back price at the first ball',
    },
    devig: {
      method: 'proportional',
      note: 'one over the price, normalised',
      market_overround: 1.006,
    },
    join: {
      quotes: 592,
      quotes_joined: 588,
      quotes_no_match: 4,
      quotes_unknown_team: 0,
      quotes_ambiguous: 0,
      label_disagreements: 0,
      key: 'exact match date and both sides through the identity layer',
    },
    formats,
  };
}

const scoredT20: MarketBenchmarkFormat = {
  matches_in_windows: 4387,
  matches_joined: 185,
  joined_share: 0.0422,
  folds: [],
  pooled: {
    n: 185,
    market_auc: 0.608,
    market_brier: 0.2387,
    display_auc_mean: 0.556,
    display_brier_mean: 0.2488,
    display_toss_aware_auc: 0.536,
    display_toss_aware_brier: 0.2514,
    market_minus_display_auc: 0.052,
    market_minus_display_brier: -0.0101,
    market_minus_toss_aware_auc: 0.0718,
    market_minus_display_auc_ci95: [-0.0197, 0.1281],
    market_minus_toss_aware_auc_ci95: [-0.0033, 0.1522],
  },
  locked: { n: 0, note: 'no joined closing price falls in the locked window' },
};

const uncoveredOdi: MarketBenchmarkFormat = {
  matches_in_windows: 1105,
  matches_joined: 0,
  joined_share: 0,
  folds: [],
  pooled: null,
  locked: { n: 0, note: 'no joined closing price falls in the locked window' },
};

describe('EvaluationMarketBenchmark', () => {
  it('shows the three arms and the gap with its interval', () => {
    render(<EvaluationMarketBenchmark benchmark={benchmark({ T20: scoredT20 })} format="T20" />);

    expect(screen.getByText('Market (closing price)')).toBeInTheDocument();
    expect(screen.getByText('Display model, as served')).toBeInTheDocument();
    expect(screen.getByText('Display model, toss-aware')).toBeInTheDocument();
    expect(screen.getByText('+0.052 (95 % -0.020 to +0.128)')).toBeInTheDocument();
  });

  it('prints the joined coverage beside the numbers', () => {
    render(<EvaluationMarketBenchmark benchmark={benchmark({ T20: scoredT20 })} format="T20" />);

    expect(screen.getByText('coverage 4.2% — 185 of 4387 matches')).toBeInTheDocument();
    expect(screen.getByText(/Pooled over the walk-forward folds, 185 matches/)).toBeInTheDocument();
  });

  it('says a format has no market comparison rather than showing an empty table', () => {
    render(<EvaluationMarketBenchmark benchmark={benchmark({ ODI: uncoveredOdi })} format="ODI" />);

    expect(screen.getByText(/No closing price joined to any ODI match/)).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });

  it('renders nothing when the report predates the benchmark', () => {
    const { container } = render(<EvaluationMarketBenchmark benchmark={undefined} format="T20" />);

    expect(container).toBeEmptyDOMElement();
  });

  it('renders nothing for a format the benchmark does not carry', () => {
    const { container } = render(
      <EvaluationMarketBenchmark benchmark={benchmark({ T20: scoredT20 })} format="TEST" />,
    );

    expect(container).toBeEmptyDOMElement();
  });
});
