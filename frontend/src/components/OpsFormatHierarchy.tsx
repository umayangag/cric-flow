import React from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Tooltip from '@mui/material/Tooltip';

type FormatHierarchyNode = {
  code: string;
  name: string;
  children?: FormatHierarchyNode[];
};

interface Props {
  hierarchy?: FormatHierarchyNode[];
}

const NodeBox: React.FC<{ node: FormatHierarchyNode; level: number }> = ({ node, level }) => {
  const isBucket = node.code === 'T20' && level === 0;

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', position: 'relative' }}>
      <Tooltip title={`Code: ${node.code}`}>
        <Paper
          elevation={2}
          sx={{
            p: 1.5,
            minWidth: 140,
            textAlign: 'center',
            bgcolor: isBucket ? 'primary.light' : 'background.paper',
            color: isBucket ? 'primary.contrastText' : 'text.primary',
            border: '1px solid',
            borderColor: 'divider',
            borderRadius: 2,
            zIndex: 1,
            mb: node.children && node.children.length > 0 ? 4 : 0,
          }}
        >
          <Typography variant="subtitle2" sx={{ fontWeight: 'bold' }}>
            {node.name}
          </Typography>
          <Typography variant="caption" sx={{ opacity: 0.8 }}>
            {node.code}
          </Typography>
        </Paper>
      </Tooltip>

      {node.children && node.children.length > 0 && (
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'row',
            gap: 4,
            position: 'relative',
            '&::before': {
              content: '""',
              position: 'absolute',
              top: -32,
              left: '50%',
              width: 1,
              height: 32,
              bgcolor: 'divider',
            },
          }}
        >
          {node.children.length > 1 && (
             <Box
               sx={{
                 position: 'absolute',
                 top: -32,
                 left: `${100 / (node.children.length * 2)}%`,
                 right: `${100 / (node.children.length * 2)}%`,
                 height: 1,
                 bgcolor: 'divider',
               }}
             />
          )}
          {node.children.map((child, idx) => (
            <NodeBox key={`${child.code}-${idx}`} node={child} level={level + 1} />
          ))}
        </Box>
      )}
    </Box>
  );
};

const OpsFormatHierarchy: React.FC<Props> = ({ hierarchy }) => {
  if (!hierarchy || hierarchy.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        Hierarchy data not available.
      </Typography>
    );
  }

  return (
    <Box
      sx={{
        p: 3,
        overflowX: 'auto',
        display: 'flex',
        justifyContent: 'center',
        bgcolor: 'grey.50',
        borderRadius: 1,
      }}
    >
      <Stack direction="row" spacing={8} alignItems="flex-start">
        {hierarchy.map((rootNode) => (
          <NodeBox key={rootNode.code} node={rootNode} level={0} />
        ))}
      </Stack>
    </Box>
  );
};

export default OpsFormatHierarchy;
