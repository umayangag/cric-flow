import { describe, it, expect, beforeEach, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { MetricGlossaryProvider, useMetricGlossary } from './MetricGlossaryContext';
import type { MetricGlossary } from '../types';

const mockMetricGlossary = vi.fn();
vi.mock('../api', () => ({
  api: {
    metricGlossary: (...args: unknown[]) => mockMetricGlossary(...args),
  },
}));

const glossary: MetricGlossary = {
  entries: {
    pinball: {
      key: 'pinball',
      name: 'Pinball loss',
      explanation: 'The proper score for a quantile forecast.',
      band: 'Only meaningful against the career-quantile baseline beside it.',
      better: 'lower is better',
      direction: 'lower',
    },
  },
};

function wrapper(enabled = true) {
  // eslint-disable-next-line react/prop-types -- test wrapper; children is typed by React
  const Wrapper = ({ children }: { children: React.ReactNode }) => (
    <MetricGlossaryProvider enabled={enabled}>{children}</MetricGlossaryProvider>
  );
  Wrapper.displayName = 'GlossaryWrapper';
  return Wrapper;
}

describe('MetricGlossaryContext', () => {
  beforeEach(() => {
    mockMetricGlossary.mockReset();
    mockMetricGlossary.mockResolvedValue(glossary);
  });

  it('resolves a metric key against the served glossary', async () => {
    const { result } = renderHook(() => useMetricGlossary(), { wrapper: wrapper() });

    await waitFor(() => expect(result.current('pinball')).toBeDefined());
    expect(result.current('pinball')?.name).toBe('Pinball loss');
    expect(result.current('pinball')?.better).toBe('lower is better');
  });

  it('resolves nothing for an unknown key, and for no key at all', async () => {
    const { result } = renderHook(() => useMetricGlossary(), { wrapper: wrapper() });

    await waitFor(() => expect(mockMetricGlossary).toHaveBeenCalled());
    expect(result.current('brand_new_score')).toBeUndefined();
    expect(result.current(undefined)).toBeUndefined();
  });

  it('does not fetch the glossary before the user has logged in', () => {
    renderHook(() => useMetricGlossary(), { wrapper: wrapper(false) });

    expect(mockMetricGlossary).not.toHaveBeenCalled();
  });

  it('resolves nothing outside a provider, so a surface still renders its numbers', () => {
    const { result } = renderHook(() => useMetricGlossary());

    expect(result.current('pinball')).toBeUndefined();
  });
});
