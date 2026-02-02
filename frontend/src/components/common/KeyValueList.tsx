import React from 'react';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';

type Item = {
  label: string;
  value: React.ReactNode;
};

type Props = {
  items: Item[];
  dense?: boolean;
};

const KeyValueList: React.FC<Props> = ({ items, dense = true }) => {
  return (
    <Stack spacing={dense ? 0.5 : 1}>
      {items.map((it, idx) => (
        <Stack key={idx} direction="row" spacing={1} alignItems="baseline">
          <Typography variant="body2" sx={{ opacity: 0.8, minWidth: 120 }}>
            {it.label}
          </Typography>
          <Typography variant="body2">{it.value}</Typography>
        </Stack>
      ))}
    </Stack>
  );
};

export default KeyValueList;
