import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import OpsRunsPanel from './OpsRunsPanel';
import type { OpsStatus, ServedFreshness } from '../utils/opsStatusHelpers';
import { UNKNOWN_FRESHNESS } from '../utils/opsStatusHelpers';

/** An /ops/status payload: the runs section, and the one freshness object beside it. */
function statusWith(artifacts: Record<string, unknown>, served?: ServedFreshness): OpsStatus {
  return {
    timestamp: '2026-09-02T10:00:00Z',
    artifacts,
    freshness: { ...UNKNOWN_FRESHNESS, served: served ?? UNKNOWN_FRESHNESS.served },
  } as unknown as OpsStatus;
}

const FRESH: ServedFreshness = {
  status: 'fresh',
  fresh: true,
  data_age_days: 2,
  max_age_days: 14,
  data_through: '2026-09-01',
  ratings_through: '2026-08-30',
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

describe('OpsRunsPanel', () => {
  it('says nothing has been trained rather than showing an empty table', () => {
    render(<OpsRunsPanel data={statusWith({ runs: [] })} />);
    expect(screen.getByText(/No runs yet/)).toBeInTheDocument();
  });

  /** H-16: which run is serving is a fact about a run id, not about files on disk. */
  it('names the loaded run and marks it in the list', () => {
    render(
      <OpsRunsPanel
        data={statusWith(
          {
            loaded_run: 'r2',
            current_run: 'r2',
            runs: [
              {
                run_id: 'r2',
                cutoff: '2025-09-01',
                ratings_through: '2026-08-30',
                git_sha: 'deadbeefcafe',
                has_manifest: true,
                refused: null,
              },
              {
                run_id: 'r1',
                cutoff: '2025-09-01',
                ratings_through: '2026-08-23',
                has_manifest: true,
                refused: null,
              },
            ],
          },
          FRESH,
        )}
      />,
    );
    expect(screen.getByText('r2')).toBeInTheDocument();
    expect(screen.getByText('r1')).toBeInTheDocument();
    expect(screen.getByText(/current$/)).toBeInTheDocument();
    expect(screen.getByText(/loaded$/)).toBeInTheDocument();
    expect(screen.getByText('deadbee')).toBeInTheDocument();
    // The verdict's last-match date and the loaded run's manifest date are one date
    // (P2-2): the badge and the loaded row's chip both read it, and the older run reads
    // its own. The verdict is taken on the boundary beside it, not on this date (SERVE-03).
    expect(screen.getByText(/last match 2026-08-30/)).toBeInTheDocument();
    expect(screen.getByText('ratings through 2026-08-30')).toBeInTheDocument();
    expect(screen.getByText('ratings through 2026-08-23')).toBeInTheDocument();
  });

  /** P2-2, §8.7: a run whose manifest predates `ratings_through` shows why it cannot be
   * loaded, not a blank where the date would be. */
  it('shows the reason a run on disk cannot be loaded', () => {
    render(
      <OpsRunsPanel
        data={statusWith(
          {
            loaded_run: 'r2',
            runs: [
              { run_id: 'r2', ratings_through: '2026-08-30', has_manifest: true, refused: null },
              {
                run_id: 'r0',
                has_manifest: true,
                refused:
                  'run r0: manifest.json carries no ratings_through, so the date its data runs through is not written down',
              },
            ],
          },
          FRESH,
        )}
      />,
    );
    expect(screen.getByText(/cannot be loaded/)).toBeInTheDocument();
    expect(
      screen.getByText(/run r0: manifest.json carries no ratings_through/),
    ).toBeInTheDocument();
    expect(screen.queryByText(/no manifest/)).not.toBeInTheDocument();
  });

  /** H-11: the verdict and the code, not a date the reader has to judge. */
  it('says live predictions are refused when the ratings are stale', () => {
    render(
      <OpsRunsPanel
        data={statusWith({ loaded_run: 'r1', runs: [{ run_id: 'r1', has_manifest: true }] }, STALE)}
      />,
    );
    expect(
      screen.getByText(/data built to 2026-07-21 — 40 days ago, past the limit of 14/),
    ).toBeInTheDocument();
    // The badge names the code and so does the sentence under it; both are the verdict's.
    expect(screen.getAllByText(/RATINGS_STALE/).length).toBeGreaterThan(0);
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
