import React from 'react';

type TileState = 'ok' | 'error' | 'neutral';

export type SimpleStatItem = {
  label: string;
  value?: string | number | boolean;
  state?: TileState;
  title?: string;
};

type Props = {
  items: SimpleStatItem[];
  size?: 'sm' | 'md';
};

const bgColor = (state: TileState | undefined): string => {
  switch (state) {
    case 'ok':
      return '#17431d';
    case 'error':
      return '#4a1010';
    default:
      return '#333';
  }
};

const tileStyle = (state: TileState | undefined, size: 'sm' | 'md'): React.CSSProperties => ({
  background: bgColor(state),
  color: '#eee',
  borderRadius: 6,
  padding: size === 'sm' ? '8px 10px' : '10px 12px',
  minWidth: size === 'sm' ? 96 : 110,
  minHeight: size === 'sm' ? 48 : 56,
  display: 'grid',
  gridTemplateRows: 'auto 1fr',
  rowGap: 4,
  border: '1px solid rgba(255,255,255,0.06)'
});

const formatValue = (v: any): string => {
  if (typeof v === 'boolean') return v ? 'yes' : 'no';
  if (typeof v === 'number') return v.toLocaleString();
  return v == null ? '—' : String(v);
};

export const SimpleStatTiles: React.FC<Props> = ({ items, size = 'md' }) => {
  const list = Array.isArray(items) ? items : [];
  return (
    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
      {list.map((it, idx) => (
        <div key={idx} style={tileStyle(it.state, size)} title={it.title || String(it.value ?? '')} aria-label={`${it.label}: ${String(it.value ?? '')}`}>
          <div style={{ fontSize: 12, opacity: 0.85 }}>{it.label}</div>
          <div style={{ fontWeight: 600 }}>{formatValue(it.value)}</div>
        </div>
      ))}
    </div>
  );
};

export default SimpleStatTiles;
