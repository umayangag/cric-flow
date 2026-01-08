import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useRouter } from 'next/router';
import AccuracyTrendChart from '../../components/AccuracyTrendChart';
import AccuracyTrendTable from '../../components/AccuracyTrendTable';
import {
  AccuracyTrendResponse,
  fetchAccuracyTrend,
  AccuracyTrendFilters,
} from '../../lib/api';

type QueryLike = Partial<AccuracyTrendFilters>;

function parseQuery(qs: Record<string, any>): QueryLike {
  const out: QueryLike = {};
  const pick = (k: keyof QueryLike) => {
    const v = qs[k as string];
    if (v !== undefined && v !== null && String(v).trim() !== '') {
      (out as any)[k] = String(v);
    }
  };
  pick('format');
  pick('team1');
  pick('team2');
  pick('start_date');
  pick('end_date');
  pick('order');
  pick('cache');
  pick('metrics');
  if (qs.limit) {
    const n = Number(qs.limit);
    if (Number.isFinite(n) && n > 0) out.limit = Math.floor(n);
  }
  return out;
}

function toQueryString(params: QueryLike): Record<string, string> {
  const out: Record<string, string> = {};
  Object.entries(params).forEach(([k, v]) => {
    if (v === undefined || v === null) return;
    const s = String(v).trim();
    if (s.length === 0) return;
    out[k] = s;
  });
  return out;
}

const defaultFilters: QueryLike = {
  order: 'asc',
  limit: 100,
  cache: 'readwrite',
  metrics: 'player,team',
};

const Page: React.FC = () => {
  const router = useRouter();
  const [filters, setFilters] = useState<QueryLike>(defaultFilters);
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string>('');
  const [data, setData] = useState<AccuracyTrendResponse | null>(null);

  // Initialize filters from URL on first render
  useEffect(() => {
    // Next.js router.query may be empty on first pass; wait until ready
    if (!router.isReady) return;
    const initial = { ...defaultFilters, ...parseQuery(router.query as any) };
    setFilters(initial);
  }, [router.isReady]);

  const doFetch = useCallback(async (p: QueryLike) => {
    setLoading(true);
    setError('');
    try {
      const res = await fetchAccuracyTrend(p as any);
      setData(res);
    } catch (e: any) {
      setError(e?.message || 'Failed to fetch');
      setData(null);
    } finally {
      setLoading(false);
    }
  }, []);

  // Fetch when filters change (after initial mount)
  useEffect(() => {
    if (!router.isReady) return;
    doFetch(filters);
  }, [filters, router.isReady, doFetch]);

  // Sync URL when filters change
  useEffect(() => {
    if (!router.isReady) return;
    const qs = toQueryString(filters);
    router.replace({ pathname: router.pathname, query: qs }, undefined, { shallow: true });
  }, [filters, router]);

  const onInput = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value } = e.target;
    setFilters((prev) => ({ ...prev, [name]: value }));
  };

  const onLimit = (e: React.ChangeEvent<HTMLInputElement>) => {
    const v = Math.max(1, Math.min(500, parseInt(e.target.value || '0', 10) || 0));
    setFilters((prev) => ({ ...prev, limit: v }));
  };

  const onMetricsToggle = (token: 'player' | 'team') => {
    setFilters((prev) => {
      const current = String(prev.metrics || '').split(',').map(s => s.trim()).filter(Boolean);
      const has = current.includes(token);
      const next = has ? current.filter(x => x !== token) : [...current, token];
      // Ensure at least one metric selected; if empty, default back to both
      const final = next.length === 0 ? ['player', 'team'] : next;
      return { ...prev, metrics: final.join(',') };
    });
  };

  const metricsState = useMemo(() => {
    const current = String(filters.metrics || '').split(',').map(s => s.trim());
    return {
      player: current.includes('player'),
      team: current.includes('team'),
    };
  }, [filters.metrics]);

  return (
    <div style={{ padding: 16 }}>
      <h1>Accuracy Trend</h1>
      <p style={{ color: '#555' }}>
        Visualize how prediction accuracy changes as we move from earlier to more recent matches. Use the filters below
        to select a format, date range, and teams. Team aggregates may use cached predictions depending on the cache mode.
      </p>

      <section style={panel}>
        <div style={row}> 
          <label style={label}>Format</label>
          <input name="format" value={filters.format || ''} onChange={onInput} placeholder="T20|ODI|TEST" />
        </div>
        <div style={row}> 
          <label style={label}>Team 1</label>
          <input name="team1" value={filters.team1 || ''} onChange={onInput} placeholder="IND" />
        </div>
        <div style={row}> 
          <label style={label}>Team 2</label>
          <input name="team2" value={filters.team2 || ''} onChange={onInput} placeholder="AUS" />
        </div>
        <div style={row}> 
          <label style={label}>Start Date</label>
          <input name="start_date" type="date" value={filters.start_date || ''} onChange={onInput} />
        </div>
        <div style={row}> 
          <label style={label}>End Date</label>
          <input name="end_date" type="date" value={filters.end_date || ''} onChange={onInput} />
        </div>
        <div style={row}> 
          <label style={label}>Order</label>
          <select name="order" value={filters.order || 'asc'} onChange={onInput}>
            <option value="asc">asc</option>
            <option value="desc">desc</option>
          </select>
        </div>
        <div style={row}> 
          <label style={label}>Limit</label>
          <input name="limit" type="number" min={1} max={500} value={filters.limit || 100} onChange={onLimit} />
        </div>
        <div style={row}> 
          <label style={label}>Cache</label>
          <select name="cache" value={(filters.cache as string) || 'readwrite'} onChange={onInput}>
            <option value="readwrite">readwrite</option>
            <option value="read">read</option>
            <option value="off">off</option>
          </select>
        </div>
        <div style={{ ...row, alignItems: 'center' }}> 
          <label style={label}>Metrics</label>
          <label style={{ marginRight: 12 }}>
            <input type="checkbox" checked={metricsState.player} onChange={() => onMetricsToggle('player')} /> player
          </label>
          <label>
            <input type="checkbox" checked={metricsState.team} onChange={() => onMetricsToggle('team')} /> team
          </label>
        </div>
        <div style={{ marginTop: 6 }}>
          <button onClick={() => doFetch(filters)} disabled={loading}>
            {loading ? 'Loading…' : 'Fetch'}
          </button>
        </div>
        {error && <div style={{ color: 'crimson', marginTop: 8 }}>{error}</div>}
      </section>

      <section style={panel}>
        <h2>Progressive Averages</h2>
        <AccuracyTrendChart data={data?.progressive || []} />
      </section>

      <section style={panel}>
        <h2>Per‑match Metrics</h2>
        <AccuracyTrendTable rows={data?.results || []} />
      </section>
    </div>
  );
};

const panel: React.CSSProperties = {
  border: '1px solid #eee',
  borderRadius: 6,
  padding: 12,
  marginTop: 12,
  background: '#fff',
};

const row: React.CSSProperties = {
  display: 'flex',
  alignItems: 'baseline',
  gap: 8,
  marginBottom: 8,
};

const label: React.CSSProperties = {
  width: 100,
  color: '#333',
};

export default Page;
