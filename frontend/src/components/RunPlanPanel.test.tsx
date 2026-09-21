import { render, screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import React from 'react';
import RunPlanPanel, { planOutcomeLabel } from './RunPlanPanel';
import type { RunPlanState } from '../types';

const mockRunPlan = vi.fn();
const mockStart = vi.fn();

vi.mock('../api', () => ({
  api: {
    opsRunPlan: (...a: unknown[]) => mockRunPlan(...a),
    opsRunPlanStart: (...a: unknown[]) => mockStart(...a),
  },
}));

const PLANS = ['full', 'import', 'retrain-only'];

const IDLE: RunPlanState = { running: false, plans: PLANS };

const RUNNING: RunPlanState = {
  id: 12,
  running: true,
  plan: 'full',
  plans: PLANS,
  started_at: '2026-08-27T12:00:00Z',
  steps: [
    { step_id: 'import', label: 'Import', status: 'COMPLETED' },
    { step_id: 'precompute', label: 'Precompute', status: 'RUNNING' },
    { step_id: 'export', label: 'Export', status: 'PENDING' },
  ],
};

const STOPPED: RunPlanState = {
  id: 12,
  running: false,
  plan: 'full',
  plans: PLANS,
  resume_from: 'precompute',
  steps: [
    { step_id: 'import', label: 'Import', status: 'COMPLETED' },
    {
      step_id: 'precompute',
      label: 'Precompute',
      status: 'FAILED',
      error: 'PRECOMPUTE_NO_ROWS: nothing to precompute — run Import first',
    },
    { step_id: 'export', label: 'Export', status: 'PENDING' },
  ],
};

describe('RunPlanPanel', () => {
  beforeEach(() => {
    mockRunPlan.mockReset();
    mockStart.mockReset();
  });
  afterEach(cleanup);

  it('starts the selected plan', async () => {
    mockRunPlan.mockResolvedValue(IDLE);
    mockStart.mockResolvedValue({ status: 202, data: { status: 'started', plan: 'full' } });
    render(<RunPlanPanel />);

    await waitFor(() => expect(screen.getByRole('button', { name: 'Run pipeline' })).toBeEnabled());
    await userEvent.click(screen.getByRole('button', { name: 'Run pipeline' }));

    await waitFor(() => expect(mockStart).toHaveBeenCalledWith({ plan: 'full' }));
  });

  // The whole point of R-1's server-side executor: the plan is the server's, so the
  // panel shows one it did not start.
  it('shows a plan started somewhere else', async () => {
    mockRunPlan.mockResolvedValue(RUNNING);
    render(<RunPlanPanel />);

    // "Plan" appears as the select's label and in the summary line, so match a step.
    await waitFor(() => expect(screen.getByText('Import')).toBeInTheDocument());
    expect(screen.getByText('Precompute')).toBeInTheDocument();
    expect(screen.getByText('RUNNING')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Running…' })).toBeDisabled();
  });

  it('counts completed steps, not merely started ones', async () => {
    mockRunPlan.mockResolvedValue(RUNNING);
    render(<RunPlanPanel />);

    await waitFor(() => expect(screen.getByText('1 / 3')).toBeInTheDocument());
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '33');
  });

  // A tooltip must describe a control, never rename it: MUI puts its title on
  // aria-label by default, which would make a screen reader announce the explanation
  // instead of the button.
  it('keeps the resume button named after what it does', async () => {
    mockRunPlan.mockResolvedValue(STOPPED);
    render(<RunPlanPanel />);

    const resume = await screen.findByRole('button', { name: 'Resume from precompute' });
    expect(resume).toHaveAccessibleDescription(/skipping what already completed/);
  });

  // Resume-from-failure without restarting from the top.
  it('offers to resume from the step that failed', async () => {
    mockRunPlan.mockResolvedValue(STOPPED);
    mockStart.mockResolvedValue({ status: 202, data: { status: 'started', resume: true } });
    render(<RunPlanPanel />);

    const resume = await screen.findByRole('button', { name: /Resume from precompute/ });
    await userEvent.click(resume);

    await waitFor(() => expect(mockStart).toHaveBeenCalledWith({ plan: 'full', resume: true }));
  });

  it('does not offer resume while the plan is running', async () => {
    mockRunPlan.mockResolvedValue(RUNNING);
    render(<RunPlanPanel />);

    await waitFor(() => expect(screen.getByText('Import')).toBeInTheDocument());
    expect(screen.queryByRole('button', { name: /Resume/ })).not.toBeInTheDocument();
  });

  it('shows why a step failed', async () => {
    mockRunPlan.mockResolvedValue(STOPPED);
    render(<RunPlanPanel />);

    await waitFor(() => expect(screen.getByText(/PRECOMPUTE_NO_ROWS/)).toBeInTheDocument());
    expect(screen.getByText(/run Import first/)).toBeInTheDocument();
  });

  // A finished plan is history. Saying so stops it being mistaken for a stalled one.
  it('says a plan is not running rather than leaving it ambiguous', async () => {
    mockRunPlan.mockResolvedValue(STOPPED);
    render(<RunPlanPanel />);

    await waitFor(() => expect(screen.getByText(/not running/)).toBeInTheDocument());
  });

  it('surfaces a refusal to start', async () => {
    mockRunPlan.mockResolvedValue(IDLE);
    mockStart.mockResolvedValue({
      status: 409,
      data: { error: 'a run plan is already in progress' },
    });
    render(<RunPlanPanel />);

    await waitFor(() => expect(screen.getByRole('button', { name: 'Run pipeline' })).toBeEnabled());
    await userEvent.click(screen.getByRole('button', { name: 'Run pipeline' }));

    expect(await screen.findByText(/already in progress/)).toBeInTheDocument();
  });

  it('offers the plans the backend named, not a hardcoded list', async () => {
    mockRunPlan.mockResolvedValue(IDLE);
    render(<RunPlanPanel />);

    await waitFor(() => expect(screen.getByLabelText('Plan')).toBeInTheDocument());
    await userEvent.click(screen.getByLabelText('Plan'));

    for (const name of PLANS) {
      expect(await screen.findByRole('option', { name })).toBeInTheDocument();
    }
  });

  // The caption used to claim optional steps are never included, which stopped being
  // true when `tune` arrived: auto-tune is the step that plan exists to run.
  it('says every plan leaves the optional step out, because they all do', async () => {
    mockRunPlan.mockResolvedValue(IDLE);
    render(<RunPlanPanel />);

    await waitFor(() => expect(screen.getByLabelText('Plan')).toBeInTheDocument());
    expect(screen.getByText(/Optional steps \(Evaluate\) are not included/)).toBeInTheDocument();

    await userEvent.click(screen.getByLabelText('Plan'));
    await userEvent.click(await screen.findByRole('option', { name: 'retrain-only' }));

    expect(
      await screen.findByText(/Build a run against data already imported/),
    ).toBeInTheDocument();
    expect(screen.getByText(/Optional steps \(Evaluate\) are not included/)).toBeInTheDocument();
  });

  it('says nothing has run rather than showing an empty list', async () => {
    mockRunPlan.mockResolvedValue(IDLE);
    render(<RunPlanPanel />);

    await waitFor(() => expect(screen.getByText(/No pipeline run yet/)).toBeInTheDocument());
    expect(screen.getByText(/closing this page does not stop it/)).toBeInTheDocument();
  });

  // A failed poll is not worth an alert: the previous state is still the best answer
  // available, and the next tick usually fixes it.
  it('keeps showing the last known plan when a poll fails', async () => {
    mockRunPlan.mockResolvedValueOnce(RUNNING).mockRejectedValue(new Error('network blip'));
    render(<RunPlanPanel />);

    await waitFor(() => expect(screen.getByText('Import')).toBeInTheDocument());
    expect(screen.queryByText(/network blip/)).not.toBeInTheDocument();
  });

  // A run somebody stopped, a run that broke and a run that finished all used to read
  // "not running", so the console could not tell an operator which had happened.
  it.each([
    ['CANCELLED', ' — stopped'],
    ['FAILED', ' — failed'],
    ['COMPLETED', ' — completed'],
  ] as const)('says how a %s plan ended', (outcome, label) => {
    expect(planOutcomeLabel({ ...STOPPED, outcome })).toBe(label);
  });

  it('says nothing about the outcome while the plan is still going', () => {
    expect(planOutcomeLabel({ ...RUNNING, outcome: 'CANCELLED' })).toBe('');
  });

  it('falls back to "not running" for a run that recorded no outcome', () => {
    expect(planOutcomeLabel(STOPPED)).toBe(' — not running');
    expect(planOutcomeLabel(null)).toBe(' — not running');
  });
});
