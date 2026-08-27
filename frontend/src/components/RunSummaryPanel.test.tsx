import { render, screen, cleanup, within } from '@testing-library/react';
import React from 'react';
import RunSummaryPanel from './RunSummaryPanel';
import type { Migration, RunMetadata } from '../types';

const META: RunMetadata = {
  step: 'train_batting',
  cutoff: '2026-01-01T00:00:00Z',
  summary: {
    saved: 2,
    formats_completed: 2,
    formats_total: 2,
    finished_at: '2026-08-26T12:05:00Z',
    formats: [
      {
        format: 'T20I',
        rows: 12345,
        features: 41,
        metrics: { rmse: 24.1 },
        artifacts: [{ path: 'batting_model_T20I.joblib', bytes: 1048576 }],
      },
      { format: 'ODI', rows: 9000, features: 41, metrics: { rmse: 31.7 } },
    ],
    dropped_columns: { T20I: ['weather_composite'] },
  },
  provenance: {
    dataset_sha256: 'abcdef0123456789',
    dataset_feed: 'all',
    dataset_source_url: 'https://cricsheet.org/downloads/all_json.zip',
    dataset_match_files: 19998,
    dataset_extracted_at: '2026-08-26T10:00:00Z',
  },
};

const PREVIOUS_RUN = {
  id: 41,
  command: 'train-batting',
  args: {},
  started_at: '2026-08-25T12:00:00Z',
  status: 'COMPLETED',
} as Migration;

describe('RunSummaryPanel', () => {
  afterEach(cleanup);

  // This is the join the whole plan was building toward: the run says which data
  // produced it.
  it('says which dataset produced the run', () => {
    render(<RunSummaryPanel metadata={META} comparisonReady />);

    expect(screen.getByText(/Dataset used/)).toBeInTheDocument();
    expect(screen.getByText(/abcdef012345/)).toBeInTheDocument();
    expect(screen.getByText(/19,998 match files/)).toBeInTheDocument();
    expect(screen.getByText(/cricsheet\.org/)).toBeInTheDocument();
  });

  // A directory populated before the registry existed genuinely has no provenance.
  // Saying so beats an empty space, and beats inventing a digest.
  it('says so plainly when provenance is unknown', () => {
    render(<RunSummaryPanel metadata={{ ...META, provenance: undefined }} comparisonReady />);

    expect(screen.getByText(/unknown/)).toBeInTheDocument();
    expect(screen.getByText(/predates the dataset registry/)).toBeInTheDocument();
  });

  it('lists each format with its rows and artifacts', () => {
    render(<RunSummaryPanel metadata={META} comparisonReady />);

    // "T20I" also appears in the dropped-columns section, so anchor on the row's
    // own unique value rather than the format name.
    const t20 = screen.getByText('12,345').closest('tr')!;
    expect(within(t20).getByText('T20I')).toBeInTheDocument();
    expect(within(t20).getByText(/batting_model_T20I\.joblib \(1\.0 MB\)/)).toBeInTheDocument();
  });

  it('shows metrics with no change when there is nothing to compare against', () => {
    render(<RunSummaryPanel metadata={META} comparisonReady />);

    expect(screen.getByText('T20I.rmse')).toBeInTheDocument();
    expect(screen.getByText(/no earlier run of this step to compare against/)).toBeInTheDocument();
  });

  it('shows the change against the previous run and names which run', () => {
    render(
      <RunSummaryPanel
        metadata={META}
        previousMetrics={{ 'T20I.rmse': 26.1, 'ODI.rmse': 30.0 }}
        previousRun={PREVIOUS_RUN}
        comparisonReady
      />,
    );

    expect(screen.getByText(/compared with run 41/)).toBeInTheDocument();
    // rmse fell by exactly 2: an improvement. Integers render unrounded.
    expect(screen.getByText('-2 (-7.7%)')).toBeInTheDocument();
    // rmse rose: a regression. Floating point leaves this fractional.
    expect(screen.getByText('+1.7000 (+5.7%)')).toBeInTheDocument();
  });

  // The plan singles this out: a silently constant feature was being dropped with
  // nobody told.
  it('names the dropped low-variance columns and says why they matter', () => {
    render(<RunSummaryPanel metadata={META} comparisonReady />);

    expect(screen.getByText(/weather_composite/)).toBeInTheDocument();
    expect(screen.getByText(/worth chasing/)).toBeInTheDocument();
  });

  it('omits the dropped-columns section when nothing was dropped', () => {
    const clean: RunMetadata = {
      ...META,
      summary: { ...META.summary, dropped_columns: {} },
    };
    render(<RunSummaryPanel metadata={clean} comparisonReady />);

    expect(screen.queryByText(/Low-variance columns dropped/)).not.toBeInTheDocument();
  });

  it('says a run recorded nothing rather than rendering an empty table', () => {
    render(<RunSummaryPanel metadata={{ step: 'train_batting' }} comparisonReady />);

    expect(screen.getByText(/recorded no per-format detail/)).toBeInTheDocument();
  });

  // Until the lookup finishes, "no earlier run" is not yet known to be true.
  it('does not claim there is no comparison while still looking', () => {
    render(<RunSummaryPanel metadata={META} comparisonReady={false} />);

    expect(
      screen.queryByText(/no earlier run of this step to compare against/),
    ).not.toBeInTheDocument();
  });
});
