import React from 'react';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';

type Props = {
  title: string;
  subtitle?: React.ReactNode;
  children?: React.ReactNode;
  // allow passing additional Paper props via sx override if ever needed
};

// SectionCard provides a uniform section layout used across Ops Status:
// Title → optional subtitle → content (tiles/matrix/details/suggestions)
const SectionCard: React.FC<Props> = ({ title, subtitle, children }) => {
  return (
    <Paper elevation={1} sx={{ p: 2, height: '100%', display: 'grid', gap: 1.25 }}>
      <Typography variant="h6" gutterBottom>
        {title}
      </Typography>
      {subtitle ? (
        <Typography variant="body2" sx={{ opacity: 0.8 }} component="div">
          {subtitle}
        </Typography>
      ) : null}
      {children}
    </Paper>
  );
};

export default SectionCard;
