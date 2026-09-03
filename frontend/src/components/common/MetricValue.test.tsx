import React from 'react';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import MetricValue from './MetricValue';
import { MetricGlossaryProvider } from '../../context/MetricGlossaryContext';
import type { MetricGlossary } from '../../types';

const mockMetricGlossary = vi.fn();
vi.mock('../../api', () => ({
  api: {
    metricGlossary: (...args: unknown[]) => mockMetricGlossary(...args),
  },
}));

const glossary: MetricGlossary = {
  entries: {
    objective_auc: {
      key: 'objective_auc',
      name: 'Objective AUC',
      explanation: 'x',
      band: '0.50 chance; 0.70-0.75 is this system.',
      better: 'higher is better',
      direction: 'higher',
      scale: { bad: 0.5, good: 0.75 },
    },
    swap_violation_share: {
      key: 'swap_violation_share',
      name: 'Swap violations',
      explanation: 'x',
      band: 'Under 2 % passes (H-4).',
      better: 'lower is better',
      direction: 'lower',
      scale: { bad: 0.02, good: 0 },
    },
    pinball: {
      key: 'pinball',
      name: 'Pinball',
      explanation: 'x',
      band: 'Only meaningful against the baseline beside it.',
      better: 'lower is better',
      direction: 'lower',
      scale: null,
    },
    width_80: {
      key: 'width_80',
      name: '10-90 interval width',
      explanation: 'x',
      band: 'Read it only beside coverage.',
      better: 'no good direction alone',
      direction: 'none',
    },
  },
};

const withGlossary = (children: React.ReactNode) => (
  <MetricGlossaryProvider>{children}</MetricGlossaryProvider>
);

describe('MetricValue', () => {
  beforeEach(() => {
    mockMetricGlossary.mockReset();
    mockMetricGlossary.mockResolvedValue(glossary);
  });

  it('paints a value at the good end of its band green, and says so in words', async () => {
    render(
      withGlossary(
        <MetricValue metricKey="objective_auc" value={{ mean: 0.75, sd: 0.01, n_folds: 12 }} />,
      ),
    );

    const painted = await screen.findByLabelText(
      '0.750 ± 0.010, strong against its reference band',
    );
    expect(painted).toHaveStyle({ color: 'rgb(22, 101, 52)' });
  });

  it('paints a value at the bad end red', async () => {
    render(withGlossary(<MetricValue metricKey="objective_auc" value={0.5} />));

    const painted = await screen.findByLabelText('0.500, poor against its reference band');
    expect(painted).toHaveStyle({ color: 'rgb(153, 27, 27)' });
  });

  it('writes a share as a percentage while reading it against the band in fractions', async () => {
    render(withGlossary(<MetricValue metricKey="swap_violation_share" value={0.02} as="share" />));

    expect(await screen.findByText('2.0%')).toBeVisible();
    expect(screen.getByLabelText('2.0%, poor against its reference band')).toBeVisible();
  });

  it('reads a metric with no band of its own against the baseline printed beside it', async () => {
    render(withGlossary(<MetricValue metricKey="pinball" value={2.4} baseline={3.0} />));

    expect(await screen.findByLabelText('2.400, strong against its reference band')).toBeVisible();
  });

  it('leaves a metric with no band and no baseline uncoloured', async () => {
    render(withGlossary(<MetricValue metricKey="width_80" value={12.4} digits={1} />));

    await waitFor(() => expect(mockMetricGlossary).toHaveBeenCalled());
    expect(screen.getByText('12.4')).toBeInTheDocument();
    expect(screen.queryByLabelText(/reference band/)).not.toBeInTheDocument();
  });

  it('paints the digits alone in the text variant, for a headline in its own tile', async () => {
    render(
      withGlossary(
        <MetricValue
          metricKey="objective_auc"
          value={0.75}
          text="0.750 ± 0.010 (n=5,504)"
          variant="text"
        />,
      ),
    );

    const painted = await screen.findByLabelText(
      '0.750 ± 0.010 (n=5,504), strong against its reference band',
    );
    expect(painted).toHaveStyle({ color: 'rgb(22, 101, 52)' });
    expect(painted).not.toHaveStyle({ backgroundColor: 'rgba(34, 197, 94, 0.18)' });
  });
});
