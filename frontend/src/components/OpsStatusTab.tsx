import React, { useCallback, useMemo, useState } from 'react';
import { api } from '../api';
import { usePolling } from '../hooks/usePolling';
import { OpsStatusSection } from './OpsStatusSection';
import type { OpsStatus } from './OpsStatusSection';

export type { OpsStatus };

const REFRESH_MS = 15000;

const OpsStatusTab: React.FC = () => {
  const [data, setData] = useState<OpsStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState<boolean>(false);

  const fetchStatus = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await api.opsStatus();
      setData(res);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to fetch /ops/status');
    } finally {
      setLoading(false);
    }
  }, []);

  // Initial load + auto-refresh while the tab is mounted.
  usePolling(fetchStatus, REFRESH_MS, true);

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
      onRefresh={fetchStatus}
    />
  );
};

export default OpsStatusTab;
