import React, { useCallback, useEffect } from 'react';
import { api } from '../api';
import { useApiCall } from '../hooks/useApiCall';
import { MLModelStatsSection } from './MLModelStatsSection';

const MLModelStatsTab: React.FC = () => {
  const fetchStats = useCallback(() => api.getModelStats(), []);
  const { data, error, loading, refetch } = useApiCall(fetchStats, {
    defaultErrorMessage: 'Failed to fetch model stats',
  });

  useEffect(() => {
    void refetch();
  }, [refetch]);

  // Refresh model stats automatically when a pipeline step completes (e.g. training/auto-tune),
  // but only while this tab is mounted.
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const handler = (_event: Event) => {
      void refetch();
    };
    window.addEventListener('cric:pipeline-completed', handler);
    return () => {
      window.removeEventListener('cric:pipeline-completed', handler);
    };
  }, [refetch]);

  return <MLModelStatsSection data={data} error={error} loading={loading} onRefresh={refetch} />;
};

export default MLModelStatsTab;
