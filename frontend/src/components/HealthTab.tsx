import React, { useEffect, useState } from 'react';
import { api } from '../api';
import type { HealthResponse } from '../types';

const HealthTab: React.FC = () => {
  const [data, setData] = useState<HealthResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const load = async () => {
    try {
      setLoading(true);
      setError(null);
      const resp = await api.health();
      setData(resp);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
  }, []);

  return (
    <div>
      <div style={{ marginBottom: 8 }}>
        <button onClick={load} disabled={loading}>
          {loading ? 'Refreshing…' : 'Refresh'}
        </button>
      </div>
      {error && (
        <div style={{ color: 'red', marginBottom: 8 }}>Error: {error}</div>
      )}
      {data && (
        <pre style={{ background: '#f7f7f7', padding: 12, overflow: 'auto' }}>
{JSON.stringify(data, null, 2)}
        </pre>
      )}
    </div>
  );
};

export default HealthTab;
