import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import PredictionReadiness from './PredictionReadiness';
import type { OpsStatus } from '../utils/opsStatusHelpers';

/** An /ops/status payload with the artifacts section go-app copies from ml-service. */
function statusWith(artifacts: Record<string, unknown>): OpsStatus {
  return { timestamp: '2026-08-28T00:00:00Z', artifacts } as unknown as OpsStatus;
}

const FRESH = { fresh: true, age_days: 2, max_age_days: 14, ratings_through: '2026-08-26' };
const STALE = { fresh: false, age_days: 40, max_age_days: 14, ratings_through: '2026-07-19' };

describe('PredictionReadiness', () => {
  it('says nothing when a run is loaded and its ratings are fresh', () => {
    const { container } = render(
      <PredictionReadiness status={statusWith({ loaded_run: 'r1', ratings: FRESH })} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it('says nothing before the status is known, rather than guessing', () => {
    const { container } = render(<PredictionReadiness status={null} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('warns when nothing is loaded', () => {
    render(<PredictionReadiness status={statusWith({ loaded_run: null, runs: [] })} />);
    expect(screen.getByText(/No training run is loaded/)).toBeInTheDocument();
  });

  /**
   * D-6: a refused artifact set and a box that has never trained look the same otherwise,
   * and only one of them is something the operator has to act on differently.
   */
  it('names the reason when the artifacts on disk were refused', () => {
    render(
      <PredictionReadiness
        status={statusWith({
          loaded_run: null,
          error: 'run 20260902T101500Z-ab12cd34: bat_pos_sum has width 1024, expected 13427',
        })}
      />,
    );
    expect(screen.getByText(/expected 13427/)).toBeInTheDocument();
  });

  /** H-11: the verdict, and the code the request would be refused with. */
  it('warns that a live prediction is refused when the ratings are stale', () => {
    render(<PredictionReadiness status={statusWith({ loaded_run: 'r1', ratings: STALE })} />);
    expect(screen.getByText(/RATINGS_STALE/)).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('2026-07-19');
  });
});
