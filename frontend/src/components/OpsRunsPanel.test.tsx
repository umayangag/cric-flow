import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import OpsRunsPanel from './OpsRunsPanel';
import type { OpsStatus } from '../utils/opsStatusHelpers';

function statusWith(artifacts: Record<string, unknown>): OpsStatus {
  return { timestamp: '2026-09-02T10:00:00Z', artifacts } as unknown as OpsStatus;
}

const FRESH = { fresh: true, age_days: 2, max_age_days: 14 };
const STALE = { fresh: false, age_days: 40, max_age_days: 14 };

describe('OpsRunsPanel', () => {
  it('says nothing has been trained rather than showing an empty table', () => {
    render(<OpsRunsPanel data={statusWith({ runs: [] })} />);
    expect(screen.getByText(/No runs yet/)).toBeInTheDocument();
  });

  /** H-16: which run is serving is a fact about a run id, not about files on disk. */
  it('names the loaded run and marks it in the list', () => {
    render(
      <OpsRunsPanel
        data={statusWith({
          loaded_run: 'r2',
          current_run: 'r2',
          ratings_through: '2026-08-30',
          ratings: FRESH,
          runs: [
            { run_id: 'r2', cutoff: '2025-09-01', git_sha: 'deadbeefcafe', has_manifest: true },
            { run_id: 'r1', cutoff: '2025-09-01', has_manifest: true },
          ],
        })}
      />,
    );
    expect(screen.getByText('r2')).toBeInTheDocument();
    expect(screen.getByText('r1')).toBeInTheDocument();
    expect(screen.getByText(/current$/)).toBeInTheDocument();
    expect(screen.getByText(/loaded$/)).toBeInTheDocument();
    expect(screen.getByText('deadbee')).toBeInTheDocument();
    expect(screen.getByText(/ratings through 2026-08-30/)).toBeInTheDocument();
  });

  /** H-11: the verdict and the code, not a date the reader has to judge. */
  it('says live predictions are refused when the ratings are stale', () => {
    render(
      <OpsRunsPanel
        data={statusWith({
          loaded_run: 'r1',
          ratings_through: '2026-07-19',
          ratings: STALE,
          runs: [{ run_id: 'r1', has_manifest: true }],
        })}
      />,
    );
    expect(screen.getByText(/40 days old, limit 14/)).toBeInTheDocument();
    expect(screen.getByText(/RATINGS_STALE/)).toBeInTheDocument();
  });

  /** D-6: a refused artifact set has to be visible as refused. */
  it('shows the loader’s refusal and flags the directory that is not a run', () => {
    render(
      <OpsRunsPanel
        data={statusWith({
          loaded_run: null,
          error: 'run r1: bat_pos_sum has width 1024, expected 13427',
          runs: [{ run_id: 'r1', has_manifest: false }],
        })}
      />,
    );
    expect(screen.getByText(/expected 13427/)).toBeInTheDocument();
    expect(screen.getByText(/no manifest/)).toBeInTheDocument();
    expect(screen.getByText(/nothing loaded/)).toBeInTheDocument();
  });
});
