import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import PredictionReadiness from './PredictionReadiness';
import type { OpsFreshness, OpsStatus, ServedFreshness } from '../utils/opsStatusHelpers';
import { UNKNOWN_FRESHNESS } from '../utils/opsStatusHelpers';

/** An /ops/status payload carrying the one freshness object go-app assembles (P2-1). */
function statusWith(served: ServedFreshness, artifacts: Record<string, unknown> = {}): OpsStatus {
  const freshness: OpsFreshness = { ...UNKNOWN_FRESHNESS, served };
  return { timestamp: '2026-08-28T00:00:00Z', artifacts, freshness } as unknown as OpsStatus;
}

const FRESH: ServedFreshness = {
  status: 'fresh',
  fresh: true,
  data_age_days: 2,
  max_age_days: 14,
  data_through: '2026-08-28',
  ratings_through: '2026-08-26',
  code: null,
};
const STALE: ServedFreshness = {
  status: 'stale',
  fresh: false,
  data_age_days: 40,
  max_age_days: 14,
  data_through: '2026-07-21',
  ratings_through: '2026-07-19',
  code: 'RATINGS_STALE',
};
const NOT_LOADED: ServedFreshness = {
  status: 'not_loaded',
  fresh: false,
  data_age_days: null,
  max_age_days: 14,
  data_through: null,
  ratings_through: null,
  code: null,
};

describe('PredictionReadiness', () => {
  it('says nothing when a run is loaded and its ratings are fresh', () => {
    const { container } = render(
      <PredictionReadiness status={statusWith(FRESH, { loaded_run: 'r1' })} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it('says nothing before the status is known, rather than guessing', () => {
    const { container } = render(<PredictionReadiness status={null} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('warns when nothing is loaded', () => {
    render(<PredictionReadiness status={statusWith(NOT_LOADED, { loaded_run: null })} />);
    expect(screen.getByText(/No training run is loaded/)).toBeInTheDocument();
  });

  /**
   * D-6: a refused artifact set and a box that has never trained look the same otherwise,
   * and only one of them is something the operator has to act on differently.
   */
  it('names the reason when the artifacts on disk were refused', () => {
    render(
      <PredictionReadiness
        status={statusWith(NOT_LOADED, {
          loaded_run: null,
          error: 'run 20260902T101500Z-ab12cd34: bat_pos_sum has width 1024, expected 13427',
        })}
      />,
    );
    expect(screen.getByText(/expected 13427/)).toBeInTheDocument();
  });

  /** H-11: the verdict, and the code the request would be refused with. */
  it('warns that a live prediction is refused when the ratings are stale', () => {
    render(<PredictionReadiness status={statusWith(STALE, { loaded_run: 'r1' })} />);
    expect(screen.getByText(/RATINGS_STALE/)).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('2026-07-19');
  });

  /**
   * §8.7: no verdict is not the same as a bad verdict. With ml-service silent the notice
   * says the answer is unknown rather than claiming nothing is loaded.
   */
  it('says the verdict is unknown when the ML service did not answer', () => {
    render(<PredictionReadiness status={statusWith(UNKNOWN_FRESHNESS.served)} />);
    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent(/did not answer/i);
    expect(alert).not.toHaveTextContent(/No training run is loaded/);
  });
});
