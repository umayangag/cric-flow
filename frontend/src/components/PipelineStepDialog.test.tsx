import { render, screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import React from 'react';
import PipelineStepDialog from './PipelineStepDialog';
import type { PipelineStep } from '../utils/pipelineSteps';

const mockPipelineRun = vi.fn();

vi.mock('../api', () => ({
  api: {
    opsPipelineRun: (...a: unknown[]) => mockPipelineRun(...a),
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

const RETRAIN_STEP: PipelineStep = {
  id: 'retrain',
  label: 'Retrain',
  status: 'pending',
  command: 'make retrain CUTOFF=2025-09-01',
  migrationCommand: 'xi-retrain',
  description: 'Rating pass, XI win models, performance models, report and run manifest.',
  runnable: true,
};

const RELOAD_STEP: PipelineStep = {
  id: 'reload',
  label: 'Reload',
  status: 'pending',
  command: 'make reload',
  migrationCommand: 'xi-reload',
  description: 'Point current at a run and load it.',
  runnable: true,
};

const REDOWNLOAD_LABEL = /Re-download the archive/i;

describe('PipelineStepDialog', () => {
  beforeEach(() => {
    mockPipelineRun.mockReset();
    mockPipelineRun.mockResolvedValue({ status: 202, data: { status: 'started' } });
  });
  afterEach(cleanup);

  it('offers the re-download option on the import step only', () => {
    render(<PipelineStepDialog step={IMPORT_STEP} onClose={vi.fn()} />);
    expect(screen.getByLabelText(REDOWNLOAD_LABEL)).not.toBeChecked();

    cleanup();
    render(<PipelineStepDialog step={RETRAIN_STEP} onClose={vi.fn()} />);
    expect(screen.queryByLabelText(REDOWNLOAD_LABEL)).toBeNull();
  });

  it('sends the cutoff a retrain was given, and nothing when it was given none', async () => {
    render(<PipelineStepDialog step={RETRAIN_STEP} onClose={vi.fn()} />);

    await userEvent.click(screen.getByRole('button', { name: 'Run' }));
    await waitFor(() => expect(mockPipelineRun).toHaveBeenCalledWith('retrain', undefined));

    await userEvent.type(screen.getByLabelText(/Cutoff/i), '2025-09-01');
    await userEvent.click(screen.getByRole('button', { name: 'Run' }));
    await waitFor(() =>
      expect(mockPipelineRun).toHaveBeenLastCalledWith('retrain', { cutoff: '2025-09-01' }),
    );
  });

  it('sends the run id a reload was given, so an operator can swap back to an earlier run', async () => {
    render(<PipelineStepDialog step={RELOAD_STEP} onClose={vi.fn()} />);

    await userEvent.type(screen.getByLabelText(/Run id/i), '20260902T101500Z-ab12cd34');
    await userEvent.click(screen.getByRole('button', { name: 'Run' }));

    await waitFor(() =>
      expect(mockPipelineRun).toHaveBeenCalledWith('reload', {
        run_id: '20260902T101500Z-ab12cd34',
      }),
    );
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
