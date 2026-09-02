import React, { createContext, useContext, useMemo, type ReactNode } from 'react';
import { api } from '../api';
import { useAsync } from '../hooks/useAsync';
import type { MetricGlossaryEntry } from '../types';

/**
 * L-1: the one place the app learns what a metric means.
 *
 * The glossary is fetched once for the whole session and handed to every surface that
 * shows a number, because the alternative — each component carrying its own sentence
 * about AUC — is how the same metric came to be described three different ways and how
 * a reference band could drift away from what the harness actually measured. Nothing
 * here writes prose: it resolves a key against what `ml/xi/glossary.py` served.
 *
 * A failed fetch is not an error state anyone needs to see. The explainers simply do not
 * appear, and every number is still rendered.
 */
export type MetricGlossaryLookup = (key: string | undefined) => MetricGlossaryEntry | undefined;

const MetricGlossaryContext = createContext<MetricGlossaryLookup>(() => undefined);

export const MetricGlossaryProvider: React.FC<{ children: ReactNode; enabled?: boolean }> = ({
  children,
  enabled = true,
}) => {
  const glossary = useAsync(api.metricGlossary, enabled ? { runOnMount: [] } : undefined);
  const entries = glossary.data?.entries;

  const lookup = useMemo<MetricGlossaryLookup>(
    () => (key) => (key == null ? undefined : entries?.[key]),
    [entries],
  );

  return <MetricGlossaryContext.Provider value={lookup}>{children}</MetricGlossaryContext.Provider>;
};

/**
 * Resolve a metric key against the served glossary.
 *
 * Outside a provider it resolves nothing, which is deliberate: a component under test,
 * or one rendered before the glossary arrives, shows its numbers without explainers
 * rather than throwing.
 */
export function useMetricGlossary(): MetricGlossaryLookup {
  return useContext(MetricGlossaryContext);
}
