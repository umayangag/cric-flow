import React, { useCallback, useMemo } from 'react';
import { api } from '../api';
import { useApiCall } from '../hooks/useApiCall';
import { usePolling } from '../hooks/usePolling';
import { OpsStatusSection } from './OpsStatusSection';
import type { OpsStatus } from './OpsStatusSection';

export type { OpsStatus };

const REFRESH_MS = 15000;

const OpsStatusTab: React.FC = () => {
  const fetchStatus = useCallback(() => api.opsStatus(), []);
  const { data, error, loading, refetch } = useApiCall(fetchStatus, {
    defaultErrorMessage: 'Failed to fetch /ops/status',
  });

  // Initial load + auto-refresh while the tab is mounted.
  usePolling(refetch, REFRESH_MS, true);

  const lastUpdated = useMemo(() => {
    if (!data?.timestamp) return '';
    try {
      const d = new Date(data.timestamp);
      return isNaN(d.getTime()) ? String(data.timestamp) : d.toLocaleString();
    } catch {
      return String(data.timestamp);
    }
  }, [data]);

  return (
    <OpsStatusSection
      data={data}
      error={error}
      loading={loading}
      lastUpdated={lastUpdated}
      onRefresh={refetch}
    />
  );
};

export default OpsStatusTab;
