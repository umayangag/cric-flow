import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import WorkbenchRunSection from './WorkbenchRunSection';
import { UNKNOWN_FRESHNESS } from '../utils/opsStatusHelpers';
import type { XiStatusResponse } from '../types';

const SERVING_RUN = '20260907T062657Z-6b16045e';
const REFUSAL = 'run 20260903T154222Z-4e009a52: manifest.json carries no ratings_through';

const LOADED: XiStatusResponse = {
  loaded: true,
  formats: ['T20'],
  players: 3,
  ratings_through: '2026-09-02',
  run_id: SERVING_RUN,
  manifest: null,
  error: null,
};

function renderWith(status: XiStatusResponse) {
  return render(
    <WorkbenchRunSection
      status={status}
      freshness={UNKNOWN_FRESHNESS}
      loading={false}
      error={null}
    />,
  );
}

describe('WorkbenchRunSection', () => {
  it('names the run still serving beside a refused reload (B-13)', () => {
    renderWith({ ...LOADED, error: REFUSAL });

    expect(
      screen.getByText(
        `The last reload was refused and ${SERVING_RUN} is still serving: ${REFUSAL}`,
      ),
    ).toBeInTheDocument();
    expect(screen.getByText(SERVING_RUN)).toBeInTheDocument();
  });

  it('shows no refusal when the last reload succeeded', () => {
    renderWith(LOADED);

    expect(screen.queryByText(/The last reload was refused/)).not.toBeInTheDocument();
    expect(screen.getByText(SERVING_RUN)).toBeInTheDocument();
  });

  it('says the artifacts on disk were refused when nothing is loaded', () => {
    renderWith({ loaded: false, formats: [], players: 0, ratings_through: null, error: REFUSAL });

    expect(screen.getByText(`The artifacts on disk were refused: ${REFUSAL}`)).toBeInTheDocument();
  });
});
