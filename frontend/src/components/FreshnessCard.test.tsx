import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import FreshnessCard from './FreshnessCard';
import PredictionReadiness from './PredictionReadiness';
import { readFreshness } from '../utils/opsStatusHelpers';
import type { OpsStatus } from '../utils/opsStatusHelpers';

const FORMATS = ['TEST', 'ODI', 'T20I', 'T20'];

/**
 * The P0-4 state on the wire: TEST's latest match is 8 days old — which the deleted 7-day
 * bucket called *stale* — while the served ratings are 2 days of 14, which H-11 calls
 * *fresh*.
 */
const P0_4_STATE = {
  timestamp: '2026-09-04T00:00:00Z',
  freshness: {
    served: {
      status: 'fresh',
      fresh: true,
      age_days: 2,
      max_age_days: 14,
      ratings_through: '2026-09-02',
      code: null,
    },
    database: {
      TEST: { latest_match_date: '2026-08-27', age_days: 8, match_count: 2857 },
      ODI: { latest_match_date: '2026-09-01', age_days: 3, match_count: 4835 },
      T20I: { latest_match_date: '2026-09-01', age_days: 3, match_count: 3402 },
      T20: { latest_match_date: '2026-09-02', age_days: 2, match_count: 11724 },
    },
    retrain_due: { status: 'up_to_date', days_behind: 0, latest_match_date: '2026-09-02' },
  },
} as unknown as OpsStatus;

const STALE_STATE = {
  timestamp: '2026-09-07T00:00:00Z',
  artifacts: { loaded_run: '20260903T160602Z-0e1e39c2' },
  freshness: {
    served: {
      status: 'stale',
      fresh: false,
      age_days: 5,
      max_age_days: 3,
      ratings_through: '2026-09-02',
      code: 'RATINGS_STALE',
    },
    database: {
      T20: { latest_match_date: '2026-09-06', age_days: 1, match_count: 11730 },
    },
    retrain_due: {
      status: 'retrain_due',
      days_behind: 4,
      latest_match_date: '2026-09-06',
      format: 'T20',
    },
  },
} as unknown as OpsStatus;

describe('FreshnessCard', () => {
  /** One verdict; the sparse format's lag is a date and a number, with no status of its own. */
  it('shows the served verdict as the only badge, with each format’s lag as a fact', () => {
    render(<FreshnessCard freshness={readFreshness(P0_4_STATE)} formats={FORMATS} />);

    expect(screen.getByText(/ratings through 2026-09-02 \(2 days old, limit 14\)/)).toBeInTheDocument();
    expect(screen.getByText(/2026-08-27 · 8d ago · 2,857 matches/)).toBeInTheDocument();
    expect(screen.queryByText(/🕒 stale/)).not.toBeInTheDocument();
  });

  /** The B-2 state: matches imported that the served run never saw, as its own line. */
  it('says a retrain is due, with the date and the days behind', () => {
    render(<FreshnessCard freshness={readFreshness(STALE_STATE)} formats={['T20']} />);

    expect(screen.getByTestId('retrain-due')).toHaveTextContent(
      /Retrain due: the database holds matches through 2026-09-06 \(T20\), 4 days past/,
    );
  });

  it('says a retrain is not due when the served ratings run through the latest match', () => {
    render(<FreshnessCard freshness={readFreshness(P0_4_STATE)} formats={FORMATS} />);

    expect(screen.getByTestId('retrain-due')).toHaveTextContent(/Retrain not due/);
  });

  it('names why a format has no date rather than showing an empty row', () => {
    const status = {
      timestamp: '2026-09-07T00:00:00Z',
      freshness: {
        served: readFreshness(P0_4_STATE).served,
        database: {
          TEST: {
            latest_match_date: null,
            age_days: null,
            match_count: 0,
            note: 'no matches of this format have been imported',
          },
        },
        retrain_due: { status: 'unknown', days_behind: null, latest_match_date: null },
      },
    } as unknown as OpsStatus;

    render(<FreshnessCard freshness={readFreshness(status)} formats={['TEST']} />);

    expect(screen.getByText(/no matches of this format have been imported/)).toBeInTheDocument();
  });
});

/**
 * The Ops badge and the Lab's readiness notice, rendered from one payload.
 *
 * This is the disagreement P2-1 closed, asserted rather than described: two surfaces read
 * the same object, so they cannot say fresh and stale about the same box, and when the
 * verdict refuses they name the same date.
 */
describe('the freshness surfaces agree', () => {
  it('shows no readiness warning while the Ops badge reads fresh', () => {
    const { container } = render(<PredictionReadiness status={P0_4_STATE} />);
    expect(container).toBeEmptyDOMElement();

    render(<FreshnessCard freshness={readFreshness(P0_4_STATE)} formats={FORMATS} />);
    expect(screen.getByText(/✅ fresh/)).toBeInTheDocument();
  });

  it('refuses on both surfaces, naming the same date, when the verdict is stale', () => {
    render(<FreshnessCard freshness={readFreshness(STALE_STATE)} formats={['T20']} />);
    render(<PredictionReadiness status={STALE_STATE} />);

    expect(screen.getByText(/🕒 stale/)).toBeInTheDocument();
    const notice = screen.getByRole('alert');
    expect(notice).toHaveTextContent('2026-09-02');
    expect(notice).toHaveTextContent('RATINGS_STALE');
    expect(screen.getAllByText(/2026-09-02/).length).toBeGreaterThan(1);
  });
});
