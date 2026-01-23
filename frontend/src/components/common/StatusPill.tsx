import React from 'react';
import Chip from '@mui/material/Chip';

export type StatusState = 'ok' | 'error' | 'warn' | 'pending' | 'unknown';

const EMOJI: Record<StatusState, string> = {
  ok: '✅',
  error: '❌',
  warn: '⚠️',
  pending: '⏳',
  unknown: 'ℹ️',
};

const COLOR: Record<StatusState, 'success' | 'error' | 'warning' | 'default' | 'info'> = {
  ok: 'success',
  error: 'error',
  warn: 'warning',
  pending: 'info',
  unknown: 'default',
};

type Props = {
  state: StatusState;
  label?: string;
  size?: 'small' | 'medium';
};

const StatusPill: React.FC<Props> = ({ state, label, size = 'small' }) => {
  const text = label ?? state;
  return (
    <Chip
      color={COLOR[state]}
      label={`${EMOJI[state]} ${text}`}
      size={size}
      variant={state === 'unknown' ? 'outlined' : 'filled'}
    />
  );
};

export default StatusPill;
