import React, { useCallback } from 'react';
import { api } from '../api';
import { useAsync } from '../hooks/useAsync';
import BiographyCoverageTable from './BiographyCoverageTable';
import ErrorNotice from './common/ErrorNotice';

/**
 * The data-quality check X-1a puts on the ops surface: how much of the archive the
 * acquired player biographies cover, beside the dataset registry that says where the
 * archive came from.
 *
 * Fetching and rendering are split the same way the registry's are: this component owns
 * the call, the table owns the presentation, and neither knows about the other's failure
 * mode.
 */
const BiographyCoverageSection: React.FC = () => {
  const fetchCoverage = useCallback(() => api.opsBiographyCoverage(), []);
  const coverage = useAsync(fetchCoverage, {
    runOnMount: [],
    errorMessage: 'Failed to load biography coverage',
  });

  return (
    <>
      <ErrorNotice error={coverage.error} title="Could not load biography coverage" />
      <BiographyCoverageTable coverage={coverage.data ?? undefined} />
    </>
  );
};

export default BiographyCoverageSection;
