import { render, screen, cleanup } from '@testing-library/react';
import React from 'react';
import PipelineStepProgressCard from './PipelineStepProgressCard';
import type { PipelineStepProgress } from '../types';

describe('PipelineStepProgressCard — dataset steps', () => {
  afterEach(cleanup);

  it('shows bytes, rate and a determinate bar when the server declared a size', () => {
    const step: PipelineStepProgress = {
      step_id: 'fetch',
      step_label: 'Fetch Dataset',
      lane: 'data',
      elapsed_sec: 30,
      fetch: { downloaded_bytes: 524288, total_bytes: 1048576, bytes_per_sec: 262144 },
    };
    render(<PipelineStepProgressCard step={step} />);

    expect(screen.getByText(/512 KB of 1.0 MB/)).toBeInTheDocument();
    expect(screen.getByText(/256 KB\/s/)).toBeInTheDocument();
    expect(screen.getByText('50%')).toBeInTheDocument();
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '50');
  });

  // No Content-Length means there is no percentage to claim. Showing one anyway would
  // be inventing a denominator the server never sent.
  it('does not invent a percentage when the size is unknown', () => {
    const step: PipelineStepProgress = {
      step_id: 'fetch',
      fetch: { downloaded_bytes: 4096, bytes_per_sec: 1024 },
    };
    render(<PipelineStepProgressCard step={step} />);

    expect(screen.queryByText(/%/)).not.toBeInTheDocument();
    expect(screen.getByRole('progressbar')).not.toHaveAttribute('aria-valuenow');
  });

  it('shows entry counts for an extraction', () => {
    const step: PipelineStepProgress = {
      step_id: 'extract',
      step_label: 'Extract Dataset',
      extract: { entries: 5000, entries_total: 20000, bytes: 1048576 },
    };
    render(<PipelineStepProgressCard step={step} />);

    expect(screen.getByText(/5,000 of 20,000 files/)).toBeInTheDocument();
    expect(screen.getByText('25%')).toBeInTheDocument();
  });

  // A step that has started but not yet published a sample has unknown progress, and
  // unknown is not zero: "0 of 0 bytes" reads as a stalled transfer.
  it('says "starting" rather than showing zero before the first sample', () => {
    render(<PipelineStepProgressCard step={{ step_id: 'fetch', step_label: 'Fetch Dataset' }} />);

    expect(screen.getByText(/Starting…/)).toBeInTheDocument();
    expect(screen.queryByText(/0 B/)).not.toBeInTheDocument();
  });

  it('leaves non-dataset steps alone', () => {
    render(<PipelineStepProgressCard step={{ step_id: 'import', step_label: 'Import' }} />);

    expect(screen.queryByText(/Starting…/)).not.toBeInTheDocument();
    expect(screen.getByText('Import')).toBeInTheDocument();
  });
});

describe('PipelineStepProgressCard — training milestones', () => {
  afterEach(cleanup);

  it('shows phase, counters and metrics for a CV fold', () => {
    const step: PipelineStepProgress = {
      step_id: 'train_win',
      step_label: 'Train Win',
      training: {
        v: 1,
        phase: 'cv',
        current: 3,
        total: 5,
        format: 'T20I',
        metrics: { accuracy: 0.7123, rows: 12345 },
        message: 'Fold 3/5 (T20I)',
      },
    };
    render(<PipelineStepProgressCard step={step} />);

    expect(screen.getByText(/Cross-validating · T20I · 3 \/ 5/)).toBeInTheDocument();
    expect(screen.getByText(/accuracy 0\.7123/)).toBeInTheDocument();
    expect(screen.getByText(/rows 12,345/)).toBeInTheDocument();
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '60');
  });

  // The plan singles this out: a silently constant feature was being dropped with
  // nobody told. Naming the columns is the entire point of surfacing it.
  it('names the dropped low-variance columns', () => {
    render(
      <PipelineStepProgressCard
        step={{
          step_id: 'train_win',
          training: {
            phase: 'features',
            dropped_columns: ['weather_composite', 'rain'],
            metrics: { dropped: 2, kept: 40 },
          },
        }}
      />,
    );

    expect(screen.getByText(/weather_composite, rain/)).toBeInTheDocument();
  });

  it('says how many dropped columns were not listed', () => {
    render(
      <PipelineStepProgressCard
        step={{
          step_id: 'train_win',
          training: { phase: 'features', dropped_columns: ['a'], dropped_columns_truncated: 12 },
        }}
      />,
    );

    expect(screen.getByText(/\+12 more/)).toBeInTheDocument();
  });

  it('lists artifacts with their sizes', () => {
    render(
      <PipelineStepProgressCard
        step={{
          step_id: 'train_batting',
          training: {
            phase: 'artifact',
            artifacts: [{ path: 'batting_model_T20I.joblib', bytes: 1048576 }],
          },
        }}
      />,
    );

    expect(screen.getByText(/batting_model_T20I\.joblib \(1\.0 MB\)/)).toBeInTheDocument();
  });

  // Unreachable and nothing-yet both render as a blank panel otherwise, but only one
  // of them is something the operator can fix.
  it('distinguishes unreachable progress from no progress yet', () => {
    const { unmount } = render(
      <PipelineStepProgressCard
        step={{ step_id: 'train_batting', step_label: 'Train Batting', progress_unavailable: true }}
      />,
    );
    expect(screen.getByText(/ml-service could not be reached/)).toBeInTheDocument();
    expect(screen.getByText(/step is still running/)).toBeInTheDocument();
    unmount();

    render(
      <PipelineStepProgressCard step={{ step_id: 'train_batting', step_label: 'Train Batting' }} />,
    );
    expect(screen.queryByText(/could not be reached/)).not.toBeInTheDocument();
  });

  it('omits the bar when there is nothing to count', () => {
    render(
      <PipelineStepProgressCard
        step={{
          step_id: 'train_batting',
          training: { phase: 'fit', message: 'Fitting on 40 rows' },
        }}
      />,
    );

    // "Fitting" appears twice: the phase label and the message. Match the message.
    expect(screen.getByText('Fitting on 40 rows')).toBeInTheDocument();
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
  });
});
