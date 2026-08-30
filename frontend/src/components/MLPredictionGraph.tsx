import React, { useMemo } from 'react';
import ReactFlow, { Background, Edge, Node, Position, Handle } from 'reactflow';
import 'reactflow/dist/style.css';
import { Box, Paper, Typography } from '@mui/material';

type StageVariant = 'default' | 'primary' | 'success' | 'warning';

type StageNodeData = {
  label: string;
  subtitle?: string;
  variant?: StageVariant;
  bold?: boolean;
};

const stageColors: Record<StageVariant, { bgcolor: string; color: string; borderColor: string }> = {
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
  const supportsReactFlow = typeof window !== 'undefined' && 'ResizeObserver' in window;

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
        position: { x: -380, y: 140 },
        data: {
          label: 'Batting model',
          subtitle: 'per-player runs, balls, 4s/6s',
          variant: 'primary',
        },
        targetPosition: Position.Top,
        sourcePosition: Position.Bottom,
      },
      {
        id: 'bowling',
        type: 'stage',
        position: { x: -150, y: 140 },
        data: {
          label: 'Bowling model',
          subtitle: 'per-player runs conceded, wickets, econ',
          variant: 'primary',
        },
        targetPosition: Position.Top,
        sourcePosition: Position.Bottom,
      },
      {
        id: 'fielding',
        type: 'stage',
        position: { x: 80, y: 140 },
        data: {
          label: 'Fielding model',
          subtitle: 'catches, run outs, stumpings',
          variant: 'primary',
        },
        targetPosition: Position.Top,
        sourcePosition: Position.Bottom,
      },
      {
        id: 'innings',
        type: 'stage',
        position: { x: 320, y: 140 },
        data: {
          label: 'Innings model',
          subtitle: 'match-level innings 1/2 runs & wickets',
          variant: 'primary',
        },
        targetPosition: Position.Top,
        sourcePosition: Position.Bottom,
      },
      {
        id: 'players',
        type: 'stage',
        position: { x: -150, y: 280 },
        data: { label: 'Per-player predictions', subtitle: 'raw bat, bowl, field lines' },
        targetPosition: Position.Top,
        sourcePosition: Position.Bottom,
      },
      {
        id: 'reconcile',
        type: 'stage',
        position: { x: 85, y: 410 },
        data: {
          label: 'Constraint reconciliation',
          subtitle: 'player lines rescaled to the innings targets',
          variant: 'primary',
        },
        targetPosition: Position.Top,
        sourcePosition: Position.Bottom,
      },
      {
        id: 'team',
        type: 'stage',
        position: { x: 85, y: 540 },
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
        position: { x: -180, y: 670 },
        data: {
          label: 'Win model',
          subtitle: 'win probability from aggregated per-player features',
          variant: 'success',
        },
        targetPosition: Position.Top,
        sourcePosition: Position.Bottom,
      },
      {
        id: 'combination_meta',
        type: 'stage',
        position: { x: 340, y: 670 },
        data: {
          label: 'Combination meta model (optional)',
          subtitle: 'Ridge weights from backtest contributions; overrides configured score weights',
        },
        sourcePosition: Position.Bottom,
      },
      {
        id: 'feedback',
        type: 'stage',
        position: { x: -180, y: 800 },
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
        position: { x: -60, y: 930 },
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
        position: { x: 250, y: 930 },
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
        position: { x: 250, y: 1060 },
        data: {
          label: 'Outcome distributions',
          subtitle: 'win prob, P10 / P50 / P90 totals',
          variant: 'warning',
        },
        targetPosition: Position.Top,
      },
    ];

    const edges: Edge[] = [
      // Features feed the per-player models and the match-level innings model alike.
      { id: 'e-features-batting', source: 'features', target: 'batting', animated: true },
      { id: 'e-features-bowling', source: 'features', target: 'bowling', animated: true },
      { id: 'e-features-fielding', source: 'features', target: 'fielding', animated: true },
      { id: 'e-features-innings', source: 'features', target: 'innings', animated: true },
      // Per-player models -> raw player lines
      { id: 'e-batting-players', source: 'batting', target: 'players', animated: true },
      { id: 'e-bowling-players', source: 'bowling', target: 'players', animated: true },
      { id: 'e-fielding-players', source: 'fielding', target: 'players', animated: true },
      // Team totals are not a plain bottom-up sum: the innings model supplies the
      // targets that reconciliation rescales every player line onto.
      { id: 'e-players-reconcile', source: 'players', target: 'reconcile', animated: true },
      { id: 'e-innings-reconcile', source: 'innings', target: 'reconcile', animated: true },
      { id: 'e-reconcile-team', source: 'reconcile', target: 'team', animated: true },
      // Team aggregates -> deterministic path (win model + feedback)
      { id: 'e-team-win', source: 'team', target: 'win', animated: true },
      { id: 'e-win-feedback', source: 'win', target: 'feedback', animated: true },
      { id: 'e-feedback-selection', source: 'feedback', target: 'selection', animated: true },
      // The meta model only supplies the bat/bowl/field weights, so it enters at the
      // two places that score a pool: the final XI and the top-k XIs the sim samples.
      { id: 'e-meta-selection', source: 'combination_meta', target: 'selection', animated: true },
      { id: 'e-meta-sim', source: 'combination_meta', target: 'sim', animated: true },
      // Team aggregates -> Monte Carlo path
      { id: 'e-team-sim', source: 'team', target: 'sim', animated: true },
      { id: 'e-sim-simout', source: 'sim', target: 'sim_out', animated: true },
    ];

    return { nodes, edges };
  }, []);

  if (!supportsReactFlow) {
    return (
      <Box
        sx={{
          width: '100%',
          height: 260,
          bgcolor: 'grey.50',
          borderRadius: 1,
          border: '1px dashed',
          borderColor: 'divider',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          px: 2,
          textAlign: 'center',
        }}
      >
        <Typography variant="body2" color="text.secondary">
          Prediction flow diagram is available only in browsers that support ResizeObserver.
        </Typography>
      </Box>
    );
  }

  return (
    <Box
      sx={{
        width: '100%',
        height: 980,
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
        panOnScroll={false}
        zoomOnScroll={false}
        zoomOnPinch={false}
        zoomOnDoubleClick={false}
        preventScrolling={false}
      >
        <Background color="#eee" gap={16} />
      </ReactFlow>
    </Box>
  );
};

export default MLPredictionGraph;
