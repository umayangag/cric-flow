import React, { useMemo } from 'react';
import ReactFlow, { Background, Controls, Edge, Node, Position, Handle } from 'reactflow';
import 'reactflow/dist/style.css';
import Box from '@mui/material/Box';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';

type StageVariant = 'default' | 'primary' | 'success' | 'warning';

type StageNodeData = {
  label: string;
  subtitle?: string;
  variant?: StageVariant;
  bold?: boolean;
};

const stageColors: Record<
  StageVariant,
  { bgcolor: string; color: string; borderColor: string }
> = {
  default: { bgcolor: 'background.paper', color: 'text.primary', borderColor: 'divider' },
  primary: { bgcolor: 'primary.light', color: 'primary.contrastText', borderColor: 'primary.main' },
  success: { bgcolor: 'success.light', color: 'success.contrastText', borderColor: 'success.main' },
  warning: { bgcolor: 'warning.light', color: 'warning.contrastText', borderColor: 'warning.main' },
};

const StageNode: React.FC<{ data: StageNodeData }> = ({ data }) => {
  const variant = data.variant ?? 'default';
  const colors = stageColors[variant];

  return (
    <Paper
      elevation={3}
      sx={{
        px: 1.5,
        py: 1,
        minWidth: 170,
        maxWidth: 220,
        textAlign: 'center',
        bgcolor: colors.bgcolor,
        color: colors.color,
        border: '1px solid',
        borderColor: colors.borderColor,
        borderRadius: 2,
      }}
    >
      <Handle type="target" position={Position.Top} style={{ background: '#555' }} />
      <Typography
        variant="body2"
        sx={{ fontWeight: data.bold ? 700 : 600, mb: data.subtitle ? 0.25 : 0 }}
      >
        {data.label}
      </Typography>
      {data.subtitle && (
        <Typography variant="caption" sx={{ opacity: 0.85 }}>
          {data.subtitle}
        </Typography>
      )}
      <Handle type="source" position={Position.Bottom} style={{ background: '#555' }} />
    </Paper>
  );
};

const nodeTypes = {
  stage: StageNode,
};

const MLPredictionGraph: React.FC = () => {
  const { nodes, edges } = useMemo(() => {
    const nodes: Node<StageNodeData>[] = [
      {
        id: 'features',
        type: 'stage',
        position: { x: 0, y: 0 },
        data: {
          label: 'Features at cutoff',
          subtitle: 'go-app (form, consistency, weather, context)',
          bold: true,
        },
        sourcePosition: Position.Bottom,
      },
      {
        id: 'batting',
        type: 'stage',
        position: { x: -250, y: 120 },
        data: { label: 'Batting model', subtitle: 'per-player runs, balls, 4s/6s', variant: 'primary' },
        targetPosition: Position.Top,
        sourcePosition: Position.Bottom,
      },
      {
        id: 'bowling',
        type: 'stage',
        position: { x: 0, y: 120 },
        data: { label: 'Bowling model', subtitle: 'per-player runs conceded, wickets, econ', variant: 'primary' },
        targetPosition: Position.Top,
        sourcePosition: Position.Bottom,
      },
      {
        id: 'fielding',
        type: 'stage',
        position: { x: 250, y: 120 },
        data: { label: 'Fielding model (optional)', subtitle: 'catches, run outs, stumpings' },
        targetPosition: Position.Top,
        sourcePosition: Position.Bottom,
      },
      {
        id: 'players',
        type: 'stage',
        position: { x: 0, y: 240 },
        data: { label: 'Per-player scores', subtitle: 'bat, bowl, field scores per player' },
        targetPosition: Position.Top,
        sourcePosition: Position.Bottom,
      },
      {
        id: 'team',
        type: 'stage',
        position: { x: 0, y: 360 },
        data: {
          label: 'Team aggregates + Extras model',
          subtitle: 'XI totals + predicted extras',
          variant: 'primary',
        },
        targetPosition: Position.Top,
        sourcePosition: Position.Bottom,
      },
      {
        id: 'win',
        type: 'stage',
        position: { x: -260, y: 500 },
        data: {
          label: 'Win model',
          subtitle: 'probability & winner from team-level features',
          variant: 'success',
        },
        targetPosition: Position.Top,
        sourcePosition: Position.Bottom,
      },
      {
        id: 'feedback',
        type: 'stage',
        position: { x: 0, y: 500 },
        data: {
          label: 'Feedback loop',
          subtitle: 'rescale innings totals & player runs to match win probability',
          variant: 'success',
        },
        targetPosition: Position.Top,
        sourcePosition: Position.Bottom,
      },
      {
        id: 'selection',
        type: 'stage',
        position: { x: 260, y: 500 },
        data: {
          label: 'Team selection & scorecard',
          subtitle: 'final XI, innings totals, winner',
          variant: 'primary',
          bold: true,
        },
        targetPosition: Position.Top,
      },
      {
        id: 'sim',
        type: 'stage',
        position: { x: -80, y: 620 },
        data: {
          label: 'Monte Carlo simulation',
          subtitle: 'sample outcomes over top‑k XIs',
          variant: 'warning',
        },
        targetPosition: Position.Top,
        sourcePosition: Position.Bottom,
      },
      {
        id: 'sim_out',
        type: 'stage',
        position: { x: 260, y: 620 },
        data: {
          label: 'Outcome distributions',
          subtitle: 'win prob, P10 / P50 / P90 totals',
          variant: 'warning',
        },
        targetPosition: Position.Top,
      },
    ];

    const edges: Edge[] = [
      // Features → models
      { id: 'e-features-batting', source: 'features', target: 'batting', animated: true },
      { id: 'e-features-bowling', source: 'features', target: 'bowling', animated: true },
      { id: 'e-features-fielding', source: 'features', target: 'fielding', animated: true },
      // Models → per-player scores
      { id: 'e-batting-players', source: 'batting', target: 'players', animated: true },
      { id: 'e-bowling-players', source: 'bowling', target: 'players', animated: true },
      { id: 'e-fielding-players', source: 'fielding', target: 'players', animated: true },
      // Per-player → team aggregates
      { id: 'e-players-team', source: 'players', target: 'team', animated: true },
      // Team aggregates → deterministic path (win model + feedback)
      { id: 'e-team-win', source: 'team', target: 'win', animated: true },
      { id: 'e-win-feedback', source: 'win', target: 'feedback', animated: true },
      { id: 'e-feedback-selection', source: 'feedback', target: 'selection', animated: true },
      // Team aggregates → Monte Carlo path
      { id: 'e-team-sim', source: 'team', target: 'sim', animated: true },
      { id: 'e-sim-simout', source: 'sim', target: 'sim_out', animated: true },
    ];

    return { nodes, edges };
  }, []);

  return (
    <Box
      sx={{
        width: '100%',
        height: 520,
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
        panOnDrag
        zoomOnScroll
        zoomOnPinch
        zoomOnDoubleClick={false}
        minZoom={0.4}
        maxZoom={1.6}
      >
        <Background color="#eee" gap={16} />
        <Controls showInteractive={false} />
      </ReactFlow>
    </Box>
  );
};

export default MLPredictionGraph;

