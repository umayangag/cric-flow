import React from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';

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

const formatValue = (v: unknown): string => {
  if (typeof v === 'boolean') return v ? 'yes' : 'no';
  if (typeof v === 'number') return v.toLocaleString();
  return v == null ? '—' : String(v);
};

export const SimpleStatTiles: React.FC<Props> = ({ items, size = 'md' }) => {
  const list = Array.isArray(items) ? items : [];
  return (
    <Box sx={{ display: 'flex', gap: 1.5, flexWrap: 'wrap' }}>
      {list.map((it, idx) => (
        <Box
          key={idx}
          title={it.title || String(it.value ?? '')}
          aria-label={`${it.label}: ${String(it.value ?? '')}`}
          sx={{
            borderRadius: 2,
            px: size === 'sm' ? 1.25 : 1.5,
            py: size === 'sm' ? 1 : 1.25,
            minWidth: size === 'sm' ? 88 : 100,
            minHeight: size === 'sm' ? 44 : 52,
            display: 'grid',
            gridTemplateRows: 'auto 1fr',
            rowGap: 0.25,
            border: '1px solid',
            ...(it.state === 'ok' && {
              bgcolor: 'rgba(34, 197, 94, 0.1)',
              borderColor: 'rgba(34, 197, 94, 0.25)',
              color: 'success.dark',
            }),
            ...(it.state === 'error' && {
              bgcolor: 'rgba(239, 68, 68, 0.08)',
              borderColor: 'rgba(239, 68, 68, 0.25)',
              color: 'error.dark',
            }),
            ...(it.state === 'neutral' && {
              bgcolor: 'grey.100',
              borderColor: 'divider',
              color: 'text.secondary',
            }),
          }}
        >
          <Typography variant="caption" sx={{ fontSize: 11, opacity: 0.9 }}>
            {it.label}
          </Typography>
          <Typography variant="body2" fontWeight={600}>
            {formatValue(it.value)}
          </Typography>
        </Box>
      ))}
    </Box>
  );
};

export default SimpleStatTiles;
