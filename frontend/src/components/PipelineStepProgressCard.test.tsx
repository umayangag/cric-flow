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
