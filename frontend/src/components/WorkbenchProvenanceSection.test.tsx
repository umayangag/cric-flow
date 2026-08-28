import { render, screen, cleanup, within } from '@testing-library/react';
import React from 'react';
import WorkbenchProvenanceSection, {
  freshnessOf,
  isBehindNewestExport,
  newestExportAmong,
} from './WorkbenchProvenanceSection';
import type { MLModelStat, ModelStatsResponse } from '../types';

function model(overrides: Partial<MLModelStat>): MLModelStat {
  return {
    model_name: 'Batting',
    match_format: 'T20I',
    trained_at: '2026-08-27T12:00:00Z',
    ...overrides,
  };
}

const LIVE_MODEL = model({
  model_name: 'Batting',
  provenance: { dataset_sha256: 'abcdef0123456789', training_cutoff: '2026-01-01T00:00:00Z' },
  dataset_is_live: true,
  accuracy_display: '92%',
});

const STALE_MODEL = model({
  model_name: 'Bowling',
  provenance: { dataset_sha256: 'oldoldoldold' },
  dataset_is_live: false,
});

const UNKNOWN_MODEL = model({ model_name: 'Fielding' });

const STATS: ModelStatsResponse = {
  models_dir: '/artifacts',
  models: [LIVE_MODEL, STALE_MODEL, UNKNOWN_MODEL],
  live_dataset: {
    dataset_sha256: 'abcdef0123456789',
    dataset_feed: 'all',
    dataset_match_files: 19998,
    dataset_extracted_at: '2026-08-26T10:00:00Z',
  },
};

describe('freshnessOf', () => {
  // Absent is a third state, not a synonym for stale. A model trained before
  // provenance existed is unaccounted for, not out of date — and flagging every such
  // model as stale would be noise rather than a warning.
  it('treats an absent verdict as unknown, never as stale', () => {
    expect(freshnessOf(model({}))).toBe('unknown');
    expect(freshnessOf(model({ provenance: {} }))).toBe('unknown');
    expect(freshnessOf(model({ dataset_is_live: false }))).toBe('stale');
    expect(freshnessOf(model({ dataset_is_live: true }))).toBe('live');
  });
});

describe('WorkbenchProvenanceSection', () => {
  afterEach(cleanup);

  // The plan opened by saying this was unanswerable.
  it('says which dataset produced which model', () => {
    render(<WorkbenchProvenanceSection stats={STATS} />);

    const row = screen.getByText('Batting').closest('tr')!;
    expect(within(row).getByText('current')).toBeInTheDocument();
    expect(within(row).getByText('abcdef012345')).toBeInTheDocument();
  });

  it('names the dataset currently on the box', () => {
    render(<WorkbenchProvenanceSection stats={STATS} />);

    expect(screen.getByText(/Live dataset/)).toBeInTheDocument();
    expect(screen.getByText(/19,998 match files/)).toBeInTheDocument();
  });

  it('flags a model trained on a dataset that is no longer live', () => {
    render(<WorkbenchProvenanceSection stats={STATS} />);

    const row = screen.getByText('Bowling').closest('tr')!;
    expect(within(row).getByText('stale')).toBeInTheDocument();
    expect(screen.getByText(/1 model was trained on a different dataset/)).toBeInTheDocument();
    expect(screen.getByText(/until you re-train/)).toBeInTheDocument();
  });

  it('marks a model with no provenance as unknown, not stale', () => {
    render(<WorkbenchProvenanceSection stats={STATS} />);

    const row = screen.getByText('Fielding').closest('tr')!;
    expect(within(row).getByText('unknown')).toBeInTheDocument();
    expect(within(row).queryByText('stale')).not.toBeInTheDocument();
  });

  it('explains what unknown means rather than leaving it bare', () => {
    render(<WorkbenchProvenanceSection stats={STATS} />);

    expect(screen.getByText(/predate provenance/)).toBeInTheDocument();
    expect(screen.getByText(/not the same as being out of date/)).toBeInTheDocument();
  });

  // The cutoff varies per run and is recorded nowhere else: two models from the same
  // export with different cutoffs are different models.
  it('shows the training cutoff', () => {
    render(<WorkbenchProvenanceSection stats={STATS} />);

    const row = screen.getByText('Batting').closest('tr')!;
    const cells = within(row).getAllByRole('cell');
    expect(cells[3].textContent).not.toBe('—');
  });

  it('does not warn when every model is current', () => {
    render(<WorkbenchProvenanceSection stats={{ ...STATS, models: [LIVE_MODEL] }} />);

    expect(screen.queryByText(/different dataset/)).not.toBeInTheDocument();
  });

  // Nothing to compare against is a different situation from every model being stale,
  // and telling the operator to re-train would be the wrong advice.
  it('says there is nothing to compare against when the box has no dataset manifest', () => {
    render(
      <WorkbenchProvenanceSection stats={{ models_dir: '/artifacts', models: [UNKNOWN_MODEL] }} />,
    );

    expect(screen.getByText(/no dataset manifest/)).toBeInTheDocument();
    expect(screen.queryByText(/different dataset/)).not.toBeInTheDocument();
  });

  it('says so when there are no models at all', () => {
    render(<WorkbenchProvenanceSection stats={{ models_dir: '/artifacts', models: [] }} />);

    expect(screen.getByText('No models on disk.')).toBeInTheDocument();
  });

  it('reports a load failure instead of an empty table', () => {
    render(<WorkbenchProvenanceSection stats={null} error="ml-service unreachable" />);

    expect(screen.getByText(/ml-service unreachable/)).toBeInTheDocument();
  });

  it('shows a loading state', () => {
    render(<WorkbenchProvenanceSection stats={null} loading />);

    expect(screen.getByText(/Loading model provenance/)).toBeInTheDocument();
  });
});

/**
 * W5-1's second flag. A model can be trained on the dataset that is still live and
 * still be out of step with its siblings: two exports of the same dataset, hours
 * apart, produce different training rows. The digest cannot catch that; the export
 * timestamp the models already carry can.
 */
describe('behind the newest export', () => {
  const older = { provenance: { exported_at: '2026-08-01T00:00:00Z' } } as MLModelStat;
  const newer = { provenance: { exported_at: '2026-08-20T00:00:00Z' } } as MLModelStat;
  const unrecorded = {} as MLModelStat;

  it('finds the newest export the set knows about', () => {
    expect(newestExportAmong([older, newer, unrecorded])).toBe('2026-08-20T00:00:00Z');
    expect(newestExportAmong([unrecorded])).toBeNull();
  });

  it('flags only models from an older export', () => {
    const newest = newestExportAmong([older, newer]);
    expect(isBehindNewestExport(older, newest)).toBe(true);
    expect(isBehindNewestExport(newer, newest)).toBe(false);
  });

  it('does not flag a model that records no export', () => {
    // Unrecorded is unknown, not behind — the same rule the dataset digest follows.
    expect(isBehindNewestExport(unrecorded, '2026-08-20T00:00:00Z')).toBe(false);
  });
});
