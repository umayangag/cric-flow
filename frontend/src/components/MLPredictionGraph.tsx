import React from 'react';
import Box from '@mui/material/Box';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';

/**
 * Static graph showing how ML models connect toward the final prediction outcome.
 * Parallel models (Batting, Bowling, Fielding) are stacked vertically; rest is horizontal flow.
 * Flow: Features → [Batting, Bowling, Fielding in parallel] → Combined metrics → Performance predictor → Win model → Team selection (XI).
 */
const MLPredictionGraph: React.FC = () => {
  const nodeSx = {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 0.75,
    px: 1.5,
    py: 1,
    border: '1px solid',
    borderColor: 'divider',
    borderRadius: 1,
    bgcolor: 'background.paper',
  };

  const arrowSx = {
    color: 'text.secondary',
    fontSize: 18,
    lineHeight: 1,
    mx: 0.25,
    alignSelf: 'center',
  };

  const parallelModels = [
    { id: 'batting', label: 'Batting', sx: { ...nodeSx, borderColor: 'primary.light' }, color: 'primary.dark' },
    { id: 'bowling', label: 'Bowling', sx: { ...nodeSx, borderColor: 'secondary.light' }, color: 'secondary.dark' },
    {
      id: 'fielding',
      label: 'Fielding',
      sub: '(optional)',
      sx: { ...nodeSx, opacity: 0.9 },
      color: 'text.secondary',
    },
  ];

  const downstreamNodes = [
    { id: 'combined', label: 'Combined metrics', sx: nodeSx },
    {
      id: 'perf',
      label: 'Performance predictor',
      sx: { ...nodeSx, borderColor: 'info.light', bgcolor: 'info.50' },
      color: 'info.dark',
    },
    {
      id: 'win',
      label: 'Win model',
      sx: { ...nodeSx, borderColor: 'success.light', bgcolor: 'success.50' },
      color: 'success.dark',
    },
    {
      id: 'team',
      label: 'Team selection (XI)',
      sx: { ...nodeSx, borderColor: 'primary.main', bgcolor: 'primary.50' },
      color: 'primary.dark',
      bold: true,
    },
  ];

  return (
    <Box
      sx={{
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'center',
        gap: 0.5,
        py: 1.5,
      }}
      role="img"
      aria-label="ML prediction flow: Features to parallel Batting, Bowling, Fielding, then Combined metrics, Performance predictor, Win model, Team selection"
    >
      {/* Features */}
      <Paper component="span" elevation={0} sx={nodeSx}>
        <Typography component="span" variant="body2" fontWeight={600}>
          Features (DB)
        </Typography>
      </Paper>
      <Typography component="span" sx={arrowSx} aria-hidden>
        →
      </Typography>

      {/* Parallel models stacked vertically */}
      <Box
        component="span"
        sx={{
          display: 'inline-flex',
          flexDirection: 'column',
          gap: 0.5,
          alignItems: 'stretch',
        }}
      >
        {parallelModels.map((node) => (
          <Paper key={node.id} component="span" elevation={0} sx={node.sx}>
            <Typography
              component="span"
              variant="body2"
              fontWeight={600}
              color={node.color || 'text.primary'}
            >
              {node.label}
            </Typography>
            {node.sub && (
              <Typography component="span" variant="caption" color="text.secondary" sx={{ ml: 0.25 }}>
                {node.sub}
              </Typography>
            )}
          </Paper>
        ))}
      </Box>

      <Typography component="span" sx={arrowSx} aria-hidden>
        →
      </Typography>

      {/* Downstream: Combined → Performance predictor → Win model → Team selection */}
      {downstreamNodes.map((node, index) => (
        <React.Fragment key={node.id}>
          <Paper
            component="span"
            elevation={node.id === 'team' ? 1 : 0}
            sx={node.sx}
          >
            <Typography
              component="span"
              variant="body2"
              fontWeight={node.bold ? 700 : 600}
              color={node.color || 'text.primary'}
            >
              {node.label}
            </Typography>
          </Paper>
          {index < downstreamNodes.length - 1 && (
            <Typography component="span" sx={arrowSx} aria-hidden>
              →
            </Typography>
          )}
        </React.Fragment>
      ))}
    </Box>
  );
};

export default MLPredictionGraph;
