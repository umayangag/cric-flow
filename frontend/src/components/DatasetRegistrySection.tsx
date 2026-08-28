import React, { useCallback } from 'react';
import { api } from '../api';
import { useAsync } from '../hooks/useAsync';
import DatasetRegistryTable from './DatasetRegistryTable';
import ErrorNotice from './common/ErrorNotice';

/** How many registry rows to show. Provenance is read newest-first; older rows are history. */
const REGISTRY_LIMIT = 50;

/**
 * Every dataset this box has acquired, on the tab that acquires them.
 *
 * It used to be the bottom of a Data tab whose top half was a feed picker, a URL box
 * and an Extract button — three operator actions to get data onto the box, with the
 * tab's own closing caption admitting the last one happened elsewhere. W6-2 made
 * Import do all three, which left this: the part that was never an action, only an
 * answer to *what data is on this box and where did it come from*. That belongs next
 * to the pipeline it feeds.
 */
const DatasetRegistrySection: React.FC = () => {
  const fetchRegistry = useCallback(() => api.opsDatasets(REGISTRY_LIMIT), []);
  const registry = useAsync(fetchRegistry, {
    runOnMount: [],
    errorMessage: 'Failed to load the dataset registry',
  });

  return (
    <>
      <ErrorNotice error={registry.error} title="Could not load the dataset registry" />
      <DatasetRegistryTable registry={registry.data ?? undefined} />
    </>
  );
};

export default DatasetRegistrySection;
