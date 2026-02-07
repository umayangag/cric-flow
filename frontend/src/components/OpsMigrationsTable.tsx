import React, { useEffect, useState } from 'react';
import { Migration, fetchOpsMigrations } from '../api/client';
import StatusPill from './common/StatusPill';
import JsonCollapse from './common/JsonCollapse';

// Helper to format duration or time ago
function formatTimeAgo(dateStr: string) {
  const date = new Date(dateStr);
  const now = new Date();
  const diff = (now.getTime() - date.getTime()) / 1000;

  if (diff < 60) return `${Math.floor(diff)}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

function formatDuration(start: string, end?: string) {
  if (!end) return '-';
  const s = new Date(start).getTime();
  const e = new Date(end).getTime();
  const ms = e - s;
  if (ms < 1000) return `${ms}ms`;
  const sec = Math.floor(ms / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  return `${min}m ${sec % 60}s`;
}

const OpsMigrationsTable: React.FC = () => {
  const [migrations, setMigrations] = useState<Migration[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = async () => {
    setLoading(true);
    try {
      const data = await fetchOpsMigrations('');
      setMigrations(data);
      setError(null);
    } catch (e) {
      if (e instanceof Error) {
        setError(e.message);
      } else {
        setError(String(e));
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
    const interval = setInterval(load, 5000); // Poll every 5s
    return () => clearInterval(interval);
  }, []);

  if (loading && migrations.length === 0) return <div>Loading migrations...</div>;
  if (error) return <div style={{ color: 'red' }}>Error: {error}</div>;

  return (
    <div style={{ overflowX: 'auto', border: '1px solid #333', borderRadius: 4 }}>
      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13, textAlign: 'left' }}>
        <thead style={{ background: '#222', color: '#ccc' }}>
          <tr>
            <th style={{ padding: 8 }}>ID</th>
            <th style={{ padding: 8 }}>Command</th>
            <th style={{ padding: 8 }}>Status</th>
            <th style={{ padding: 8 }}>Started</th>
            <th style={{ padding: 8 }}>Duration</th>
            <th style={{ padding: 8 }}>Details</th>
          </tr>
        </thead>
        <tbody>
          {migrations.map((m) => (
            <tr key={m.id} style={{ borderTop: '1px solid #333' }}>
              <td style={{ padding: 8 }}>{m.id}</td>
              <td style={{ padding: 8, fontFamily: 'monospace' }}>{m.command}</td>
              <td style={{ padding: 8 }}>
                <StatusPill
                  status={
                    m.status === 'COMPLETED'
                      ? 'ok'
                      : m.status === 'IN_PROGRESS'
                        ? 'working'
                        : 'error'
                  }
                >
                  {m.status}
                </StatusPill>
              </td>
              <td style={{ padding: 8 }}>{formatTimeAgo(m.started_at)}</td>
              <td style={{ padding: 8 }}>{formatDuration(m.started_at, m.completed_at)}</td>
              <td style={{ padding: 8 }}>
                {m.error_message ? (
                  <div style={{ color: 'red', maxWidth: 300 }}>{m.error_message}</div>
                ) : (
                  <JsonCollapse label="Meta" data={{ args: m.args, meta: m.metadata }} />
                )}
              </td>
            </tr>
          ))}
          {migrations.length === 0 && (
            <tr>
              <td colSpan={6} style={{ padding: 16, textAlign: 'center', color: '#666' }}>
                No commands executed yet.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
};

export default OpsMigrationsTable;
