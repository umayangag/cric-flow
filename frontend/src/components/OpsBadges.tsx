import React from 'react';

type Services = {
  api_health?: boolean;
  api_readiness?: boolean;
  ml_health?: boolean;
};

type Props = {
  services: Services | undefined;
  timestamp?: string;
};

const pillStyle = (ok: boolean | undefined): React.CSSProperties => ({
  display: 'inline-block',
  padding: '4px 10px',
  borderRadius: 999,
  fontSize: 12,
  fontWeight: 600,
  background: ok === true ? '#17431d' : ok === false ? '#4a1010' : '#333',
  color: ok === true ? '#b2f5c1' : ok === false ? '#ffb3b3' : '#ddd',
  border: '1px solid rgba(255,255,255,0.1)'
});

export const OpsBadges: React.FC<Props> = ({ services, timestamp }) => {
  const items: Array<[string, boolean | undefined]> = [
    ['API', services?.api_health],
    ['DB Ready', services?.api_readiness],
    ['ML', services?.ml_health],
  ];
  let last = '';
  if (timestamp) {
    try {
      const d = new Date(timestamp);
      last = isNaN(d.getTime()) ? String(timestamp) : d.toLocaleString();
    } catch {
      last = String(timestamp);
    }
  }
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
      {items.map(([label, ok]) => (
        <span key={label} style={pillStyle(ok)} title={`${label}: ${ok === true ? 'ok' : ok === false ? 'unhealthy' : 'unknown'}`}>
          {label}: {ok === true ? 'OK' : ok === false ? 'DOWN' : '—'}
        </span>
      ))}
      {last && (
        <small style={{ opacity: 0.8, marginLeft: 8 }}>Last updated: {last}</small>
      )}
    </div>
  );
};

export default OpsBadges;
