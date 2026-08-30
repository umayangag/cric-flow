import React, { useState } from 'react';
import { Box, Button, Collapse } from '@mui/material';

type Props = {
  data: unknown;
  summary?: string;
  defaultOpen?: boolean;
};

const JsonCollapse: React.FC<Props> = ({
  data,
  summary = 'Show raw JSON',
  defaultOpen = false,
}) => {
  const [open, setOpen] = useState<boolean>(defaultOpen);
  return (
    <Box sx={{ mt: 1 }}>
      <Button variant="text" size="small" onClick={() => setOpen((v) => !v)} aria-expanded={open}>
        {open ? 'Hide raw JSON' : summary}
      </Button>
      <Collapse in={open} timeout={200} unmountOnExit>
        <Box
          component="pre"
          sx={{
            p: 1.5,
            bgcolor: 'grey.100',
            color: 'text.primary',
            borderRadius: 1,
            overflow: 'auto',
            border: '1px solid',
            borderColor: 'divider',
            fontSize: 12,
            fontFamily: 'ui-monospace, Menlo, monospace',
          }}
        >
          {JSON.stringify(data, null, 2)}
        </Box>
      </Collapse>
    </Box>
  );
};

export default JsonCollapse;
