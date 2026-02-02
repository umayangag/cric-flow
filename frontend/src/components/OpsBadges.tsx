import React from 'react';
import Chip from '@mui/material/Chip';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';

const emoji = (ok: boolean | undefined): string =>
  ok === true ? '✅' : ok === false ? '❌' : '⏳';
const color = (ok: boolean | undefined): 'success' | 'error' | 'default' | 'info' =>
  ok === true ? 'success' : ok === false ? 'error' : 'info';

type Services = {
  api_health?: boolean;
  api_readiness?: boolean;
  ml_health?: boolean;
};

type Props = {
  services: Services | undefined;
  timestamp?: string;
};

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
    <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
      {items.map(([label, ok]) => (
        <Chip
          key={label}
          color={color(ok)}
          size="small"
          label={`${emoji(ok)} ${label}: ${ok === true ? 'Healthy' : ok === false ? 'Down' : 'Unknown'}`}
        />
      ))}
      {last && (
        <Typography component="span" variant="caption" sx={{ opacity: 0.8, ml: 0.5 }}>
          Last updated: {last}
        </Typography>
      )}
    </Stack>
  );
};

export default OpsBadges;
