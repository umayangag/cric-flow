import React, { useState } from 'react';
import Button from '@mui/material/Button';
import Collapse from '@mui/material/Collapse';
import Box from '@mui/material/Box';

type Props = {
  data: unknown;
  summary?: string;
  defaultOpen?: boolean;
};

const JsonCollapse: React.FC<Props> = ({ data, summary = 'Show raw JSON', defaultOpen = false }) => {
  const [open, setOpen] = useState<boolean>(defaultOpen);
  return (
    <Box sx={{ mt: 1 }}>
      <Button variant="text" size="small" onClick={() => setOpen((v) => !v)} aria-expanded={open}>
        {open ? 'Hide raw JSON' : summary}
      </Button>
      <Collapse in={open} timeout={200} unmountOnExit>
        <Box component="pre" sx={{ p: 1.5, bgcolor: '#111', color: '#ddd', borderRadius: 1, overflow: 'auto' }}>
          {JSON.stringify(data, null, 2)}
        </Box>
      </Collapse>
    </Box>
  );
};

export default JsonCollapse;
