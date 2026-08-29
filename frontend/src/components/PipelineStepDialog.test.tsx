import { render, screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import React from 'react';
import PipelineStepDialog from './PipelineStepDialog';
import type { PipelineStep } from '../utils/pipelineSteps';

const mockPipelineRun = vi.fn();
const mockModelStats = vi.fn();

vi.mock('../api', () => ({
  api: {
    opsPipelineRun: (...a: unknown[]) => mockPipelineRun(...a),
    getModelStats: (...a: unknown[]) => mockModelStats(...a),
  },
}));

const IMPORT_STEP: PipelineStep = {
  id: 'import',
  label: 'Import',
  status: 'pending',
  command: 'make migrate && make cricsheet-import',
  migrationCommand: 'cricsheet-import',
  description: 'Download the configured Cricsheet archive, extract it and load the matches.',
  runnable: true,
};

const EXPORT_STEP: PipelineStep = {
  id: 'export',
  label: 'Export',
  status: 'pending',
  command: 'make export-dataset',
  migrationCommand: 'export-dataset',
  description: 'Export all-format and per-format CSVs.',
  runnable: true,
};

const REDOWNLOAD_LABEL = /Re-download the archive/i;

describe('PipelineStepDialog', () => {
  beforeEach(() => {
    mockPipelineRun.mockReset();
    mockModelStats.mockReset();
    mockPipelineRun.mockResolvedValue({ status: 202, data: { status: 'started' } });
    mockModelStats.mockResolvedValue({ models: [] });
  });
  afterEach(cleanup);

  it('offers the re-download option on the import step only', () => {
    render(<PipelineStepDialog step={IMPORT_STEP} onClose={vi.fn()} />);
    expect(screen.getByLabelText(REDOWNLOAD_LABEL)).not.toBeChecked();

    cleanup();
    render(<PipelineStepDialog step={EXPORT_STEP} onClose={vi.fn()} />);
    expect(screen.queryByLabelText(REDOWNLOAD_LABEL)).toBeNull();
  });

  it('runs import without parameters when re-download is not ticked', async () => {
    render(<PipelineStepDialog step={IMPORT_STEP} onClose={vi.fn()} />);

    await userEvent.click(screen.getByRole('button', { name: 'Run' }));

    await waitFor(() => expect(mockPipelineRun).toHaveBeenCalledWith('import', undefined));
  });

  it('sends refresh=1 when re-download is ticked', async () => {
    render(<PipelineStepDialog step={IMPORT_STEP} onClose={vi.fn()} />);

    await userEvent.click(screen.getByLabelText(REDOWNLOAD_LABEL));
    await userEvent.click(screen.getByRole('button', { name: 'Run' }));

    await waitFor(() => expect(mockPipelineRun).toHaveBeenCalledWith('import', { refresh: '1' }));
  });

  it('shows the backend reason for every step the plan skipped', async () => {
    mockPipelineRun.mockResolvedValue({
      status: 202,
      data: {
        status: 'started',
        plan: 'import',
        steps: ['fetch', 'extract', 'import'],
        skipped: {
          fetch:
            'the dataset directory already holds all_json.zip from this source (12 match files)',
          extract:
            'the dataset directory already holds all_json.zip from this source (12 match files)',
        },
      },
    });
    render(<PipelineStepDialog step={IMPORT_STEP} onClose={vi.fn()} />);

    await userEvent.click(screen.getByRole('button', { name: 'Run' }));

    expect(
      await screen.findByText(/Fetch skipped — the dataset directory already holds/),
    ).toBeTruthy();
    expect(screen.getByText(/Extract skipped — the dataset directory already holds/)).toBeTruthy();
  });

  it('reports no skips when the plan runs every step', async () => {
    render(<PipelineStepDialog step={IMPORT_STEP} onClose={vi.fn()} />);

    await userEvent.click(screen.getByRole('button', { name: 'Run' }));

    expect(await screen.findByText(/Step started/)).toBeTruthy();
    expect(screen.queryByText(/Not every step in the plan will run/)).toBeNull();
  });

  it('clears the re-download choice when the dialog is closed', async () => {
    const onClose = vi.fn();
    const { rerender } = render(<PipelineStepDialog step={IMPORT_STEP} onClose={onClose} />);

    await userEvent.click(screen.getByLabelText(REDOWNLOAD_LABEL));
    await userEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(onClose).toHaveBeenCalled();

    rerender(<PipelineStepDialog step={IMPORT_STEP} onClose={onClose} />);
    expect(screen.getByLabelText(REDOWNLOAD_LABEL)).not.toBeChecked();
  });
});
