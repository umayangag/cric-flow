import { render, screen, cleanup } from '@testing-library/react';
import React from 'react';
import BiographyCoverageTable from './BiographyCoverageTable';
import type { BiographyCoverageResponse, BiographyCoverageRow } from '../types';

const ROW: BiographyCoverageRow = {
  format: 'ODI',
  gender: 'male',
  appearances: 1000,
  attempted: 1000,
  matched: 940,
  birth_date: 931,
  batting_hand: 8,
  bowling_style: 67,
  career_end: 2,
  death: 3,
  players: 100,
  matched_players: 75,
};

const coverageWith = (
  overrides: Partial<BiographyCoverageResponse> = {},
): BiographyCoverageResponse => ({
  generated_at: '2026-09-04T07:52:26Z',
  rows: [ROW],
  total: ROW,
  unmatched: [
    {
      player_id: 9,
      name: 'CJA Amini',
      cricsheet_id: '4d84ad05',
      cricinfo_id: '332980',
      appearances: 127,
      attempted: true,
    },
  ],
  last_fetched_at: '2026-09-04T07:52:26Z',
  source_license: 'CC0-1.0',
  ...overrides,
});

const NEVER_ACQUIRED = /No biographies have been acquired on this box/i;

describe('BiographyCoverageTable', () => {
  afterEach(cleanup);

  it('shows each fact as a share of appearances, and the licence it came under', () => {
    render(<BiographyCoverageTable coverage={coverageWith()} />);

    expect(screen.getByText('ODI')).toBeTruthy();
    // 931 of 1000 appearances have a date of birth: the figure the X-1b gate reads.
    expect(screen.getAllByText('93.1%').length).toBeGreaterThan(0);
    expect(screen.getByText(/CC0-1.0/)).toBeTruthy();
  });

  it('distinguishes a pass that has never run from a source that had nothing', () => {
    render(<BiographyCoverageTable coverage={coverageWith({ last_fetched_at: undefined })} />);

    expect(screen.getByText(NEVER_ACQUIRED)).toBeTruthy();
  });

  it('does not claim the pass never ran when it has', () => {
    render(<BiographyCoverageTable coverage={coverageWith()} />);

    expect(screen.queryByText(NEVER_ACQUIRED)).toBeNull();
    expect(screen.getByText(/Acquired/)).toBeTruthy();
  });

  it('names the largest gap, so the next curated override is obvious', () => {
    render(<BiographyCoverageTable coverage={coverageWith()} />);

    expect(screen.getByText(/CJA Amini/)).toBeTruthy();
  });

  it('says the database may be down rather than rendering an empty table as zero coverage', () => {
    render(<BiographyCoverageTable />);

    expect(screen.getByText(/Coverage unavailable/i)).toBeTruthy();
    expect(screen.queryByText('ODI')).toBeNull();
  });
});
