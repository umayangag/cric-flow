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

  return (
    <MLModelStatsSection
      data={data}
      error={error}
      loading={loading}
      onRefresh={fetchStats}
    />
  );
};

export default MLModelStatsTab;
