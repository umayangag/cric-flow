import React, { useCallback, useEffect, useState } from 'react';
import { api } from '../api';
import { Suggestion } from '../types';

const codeStyle: React.CSSProperties = {
  background: '#111',
  color: '#ddd',
  padding: 12,
  borderRadius: 6,
  fontFamily: 'ui-monospace, Menlo, monospace',
  fontSize: 12,
  overflowX: 'auto',
  border: '1px solid rgba(255,255,255,0.06)',
  userSelect: 'all',
  cursor: 'text',
};

const OpsSuggestions: React.FC = () => {
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copiedIdx, setCopiedIdx] = useState<number | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api.opsSuggestions();
      setSuggestions(data);
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
  }, []);

  useEffect(() => {
    load();
    const interval = setInterval(load, 10000);
    return () => clearInterval(interval);
  }, [load]);

  const onCopy = async (idx: number, cmd: string) => {
    try {
      await navigator.clipboard.writeText(cmd);
      setCopiedIdx(idx);
      setTimeout(() => setCopiedIdx(null), 1200);
    } catch {
      setCopiedIdx(null);
      alert('Failed to copy to clipboard');
    }
  };

  if (loading && suggestions.length === 0) return <div>Loading suggestions...</div>;
  if (error) return <div style={{ color: 'red' }}>Error: {error}</div>;

  return (
    <>
      {suggestions.length === 0 ? (
        <div style={{ opacity: 0.9 }}>No suggestions.</div>
      ) : (
        <div style={{ display: 'grid', gap: 12 }}>
          {suggestions.map((s, i) => (
            <div
              key={s.command || s.title}
              style={{
                display: 'grid',
                gap: 6,
                borderLeft: `4px solid ${s.priority === 'HIGH' ? '#ff4444' : '#4488ff'}`,
                paddingLeft: 12,
                background: 'rgba(255,255,255,0.03)',
                padding: 12,
                borderRadius: 4,
              }}
            >
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  gap: 8,
                }}
              >
                <div>
                  <strong>{s.title}</strong>
                  <div style={{ fontSize: 13, color: '#aaa' }}>{s.description}</div>
                </div>
                {s.command && (
                  <button
                    type="button"
                    onClick={() => onCopy(i, s.command)}
                    title="Copy command"
                    style={{
                      padding: '4px 8px',
                      background: '#333',
                      color: '#fff',
                      border: 'none',
                      borderRadius: 4,
                      cursor: 'pointer',
                    }}
                  >
                    {copiedIdx === i ? 'Copied!' : 'Copy'}
                  </button>
                )}
              </div>
              {s.command && <pre style={codeStyle}>{s.command}</pre>}
            </div>
          ))}
        </div>
      )}
    </>
  );
};

export default OpsSuggestions;
