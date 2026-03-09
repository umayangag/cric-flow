import React, { useCallback, useEffect, useState } from 'react';
import { api } from '../api';
import type { ModelStatsResponse } from '../types';
import { MLModelStatsSection } from './MLModelStatsSection';

const MLModelStatsTab: React.FC = () => {
  const [data, setData] = useState<ModelStatsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const fetchStats = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await api.getModelStats();
      setData(res);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to fetch model stats');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchStats();
  }, [fetchStats]);

  // Refresh model stats automatically when a pipeline step completes (e.g. training/auto-tune),
  // but only while this tab is mounted.
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const handler = () => {
      void fetchStats();
    };
    window.addEventListener('cric:pipeline-completed', handler as EventListener);
    return () => {
      window.removeEventListener('cric:pipeline-completed', handler as EventListener);
    };
  }, [fetchStats]);

  return <MLModelStatsSection data={data} error={error} loading={loading} onRefresh={fetchStats} />;
};

export default MLModelStatsTab;
