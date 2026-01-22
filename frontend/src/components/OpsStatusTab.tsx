import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { api } from '../api';
import OpsBadges from './OpsBadges';
import OpsMatrix from './OpsMatrix';
import OpsSuggestions from './OpsSuggestions';

type OpsStatus = any; // Contract documented in docs/ops-status.md; keep loose for forward compatibility

const REFRESH_MS = 15000;

const OpsStatusTab: React.FC = () => {
  const [data, setData] = useState<OpsStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState<boolean>(false);
  const timerRef = useRef<number | null>(null);

  const fetchStatus = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await api.opsStatus();
      setData(res);
    } catch (e: any) {
      setError(e?.message ?? 'Failed to fetch /ops/status');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchStatus();
    timerRef.current = window.setInterval(fetchStatus, REFRESH_MS);
    return () => {
      if (timerRef.current) {
        window.clearInterval(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [fetchStatus]);

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
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
        <button
          type="button"
          onClick={fetchStatus}
          disabled={loading}
          title="Refresh Ops Status"
          aria-label="Refresh Ops Status"
        >
          {loading ? 'Refreshing…' : 'Refresh'}
        </button>
        <small>
          Auto-refresh: {REFRESH_MS / 1000}s{lastUpdated ? ` • Last updated: ${lastUpdated}` : ''}
        </small>
      </div>

      {error && (
        <div style={{ color: 'red', marginBottom: 12 }} role="alert" aria-live="polite">
          Error: {error}
        </div>
      )}

      {!data && !error && (
        <div>
          <em>Loading Ops Status…</em>
        </div>
      )}

      {data && (
        <div style={{ display: 'grid', gap: 16 }}>
          <section>
            <h3 style={{ margin: '8px 0' }}>Services</h3>
            <OpsBadges services={data.services} timestamp={data.timestamp} />
          </section>

          <section>
            <h3 style={{ margin: '8px 0' }}>Database</h3>
            <pre style={{ background: '#111', color: '#ddd', padding: 12, borderRadius: 6 }}>
              {JSON.stringify(data.db, null, 2)}
            </pre>
          </section>

          <OpsMatrix type="precompute" title="Precompute" data={data.precompute} />

          <OpsMatrix type="exports" title="Exports" data={data.exports} />

          <OpsMatrix type="artifacts" title="Artifacts" data={data.artifacts} />

          <OpsSuggestions suggestions={data.suggestions} />

          <details>
            <summary>Raw payload (debug)</summary>
            <pre style={{ background: '#111', color: '#ddd', padding: 12, borderRadius: 6 }}>
              {JSON.stringify(data, null, 2)}
            </pre>
          </details>
        </div>
      )}
    </div>
  );
};

export default OpsStatusTab;
