import React, { useMemo } from 'react';
import { FormatHierarchyNode } from '../types';
import ReactFlow, { Node, Edge, Background, ConnectionLineType, Position, Handle } from 'reactflow';
import 'reactflow/dist/style.css';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Paper from '@mui/material/Paper';

interface Props {
  hierarchy?: FormatHierarchyNode[];
}

// Custom Node component to use MUI Paper
const CustomNode = ({ data }: { data: { label: string; code: string; isBucket: boolean } }) => {
  return (
    <Paper
      elevation={3}
      sx={{
        p: 1.5,
        minWidth: 150,
        textAlign: 'center',
        bgcolor: data.isBucket ? 'primary.light' : 'background.paper',
        color: data.isBucket ? 'primary.contrastText' : 'text.primary',
        border: '1px solid',
        borderColor: 'divider',
        borderRadius: 2,
      }}
    >
      <Handle type="target" position={Position.Top} style={{ background: '#555' }} />
      <Typography variant="subtitle2" sx={{ fontWeight: 'bold' }}>
        {data.label}
      </Typography>
      <Typography variant="caption" sx={{ opacity: 0.8 }}>
        {data.code}
      </Typography>
      <Handle type="source" position={Position.Bottom} style={{ background: '#555' }} />
    </Paper>
  );
};

const nodeTypes = {
  custom: CustomNode,
};

const OpsFormatHierarchy: React.FC<Props> = ({ hierarchy }) => {
  const { nodes, edges } = useMemo(() => {
    const initialNodes: Node[] = [];
    const initialEdges: Edge[] = [];

    if (!hierarchy) return { nodes: initialNodes, edges: initialEdges };

    const HORIZONTAL_SPACING = 250;
    const VERTICAL_SPACING = 150;

    const flatten = (
      node: FormatHierarchyNode,
      x: number,
      y: number,
      parentId?: string,
      level: number = 0,
    ) => {
      const id = parentId ? `${parentId}-${node.code}-${level}` : `${node.code}-${level}`;

      initialNodes.push({
        id,
        type: 'custom',
        data: {
          label: node.name,
          code: node.code,
          isBucket: node.code === 'T20' && level === 0,
        },
        position: { x, y },
      });

      if (parentId) {
        initialEdges.push({
          id: `e-${parentId}-${id}`,
          source: parentId,
          target: id,
          type: ConnectionLineType.SmoothStep,
          animated: true,
          style: { stroke: '#999' },
        });
      }

      if (node.children) {
        const totalWidth = (node.children.length - 1) * HORIZONTAL_SPACING;
        node.children.forEach((child, index) => {
          const childX = x - totalWidth / 2 + index * HORIZONTAL_SPACING;
          flatten(child, childX, y + VERTICAL_SPACING, id, level + 1);
        });
      }
    };

    hierarchy.forEach((root, index) => {
      flatten(root, index * HORIZONTAL_SPACING * 1.5, 0);
    });

    return { nodes: initialNodes, edges: initialEdges };
  }, [hierarchy]);

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
        width: '100%',
        height: 400,
        bgcolor: 'grey.50',
        borderRadius: 1,
        border: '1px solid',
        borderColor: 'divider',
      }}
    >
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        fitView
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
        panOnDrag={false}
        zoomOnScroll={false}
        zoomOnPinch={false}
        zoomOnDoubleClick={false}
      >
        <Background color="#aaa" gap={16} />
      </ReactFlow>
    </Box>
  );
};

export default OpsFormatHierarchy;
