import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MetricInfo, MetricLabel } from './MetricInfo';
import { MetricGlossaryProvider } from '../../context/MetricGlossaryContext';
import EvaluationPerformanceTable from '../EvaluationPerformance';
import type { EvaluationPerformance, MetricGlossary } from '../../types';

const mockMetricGlossary = vi.fn();
vi.mock('../../api', () => ({
  api: {
    metricGlossary: (...args: unknown[]) => mockMetricGlossary(...args),
  },
}));

const glossary: MetricGlossary = {
  entries: {
    coverage_80: {
      key: 'coverage_80',
      name: '10-90 coverage',
      explanation: 'How often reality landed inside the stated 80 % range.',
      band: 'Nominal 0.80; within +/- 0.03 is calibrated (H-5).',
      better: 'closer to the nominal is better',
      direction: 'nominal',
    },
  },
};

const withGlossary = (children: React.ReactNode) => (
  <MetricGlossaryProvider>{children}</MetricGlossaryProvider>
);

describe('MetricInfo', () => {
  beforeEach(() => {
    mockMetricGlossary.mockReset();
    mockMetricGlossary.mockResolvedValue(glossary);
  });

  it('opens the explainer with the measured value placed on the reference band', async () => {
    render(withGlossary(<MetricInfo metricKey="coverage_80" value="0.786" />));

    const button = await screen.findByRole('button', { name: 'What is 10-90 coverage?' });
    await userEvent.click(button);

    expect(
      screen.getByText('How often reality landed inside the stated 80 % range.'),
    ).toBeVisible();
    expect(screen.getByText('0.786', { exact: false })).toBeVisible();
    expect(
      screen.getByText('Nominal 0.80; within +/- 0.03 is calibrated (H-5).', { exact: false }),
    ).toBeVisible();
    expect(screen.getByText('closer to the nominal is better', { exact: false })).toBeVisible();
  });

  it('renders the label and no explainer for a key the glossary does not carry', async () => {
    render(withGlossary(<MetricLabel metricKey="not_a_metric" label="Mystery" />));

    await waitFor(() => expect(mockMetricGlossary).toHaveBeenCalled());
    expect(screen.getByText('Mystery')).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('shows every number without explainers when the glossary cannot be fetched', async () => {
    mockMetricGlossary.mockRejectedValue(new Error('no glossary'));
    render(withGlossary(<MetricLabel metricKey="coverage_80" label="10–90 coverage" />));

    await waitFor(() => expect(mockMetricGlossary).toHaveBeenCalled());
    expect(screen.getByText('10–90 coverage')).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('explains a metric label in the evaluation tables it is wired into', async () => {
    const performance = {
      targets: {
        runs: { model: { interval: { coverage_80: 0.81, width_80: 30.2 } } },
      },
    } as unknown as EvaluationPerformance;

    render(
      withGlossary(
        <EvaluationPerformanceTable
          performance={performance}
          title="Performance, locked window"
          caption="Scored once per release."
        />,
      ),
    );

    expect(
      await screen.findByRole('button', { name: 'What is 10–90 coverage?' }),
    ).toBeInTheDocument();
  });
});
