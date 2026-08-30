import React from 'react';
import { Box, Button, Paper, Stack, Typography } from '@mui/material';
import { describeStartupError } from '../../lib/startupDiagnostics';

interface RootErrorBoundaryProps {
  children: React.ReactNode;
}

interface RootErrorBoundaryState {
  error: Error | null;
}

/**
 * Catches render-time throws so a broken subtree shows the error instead of an
 * empty page. Module-evaluation failures happen before React mounts and cannot
 * reach this boundary — those are handled by the bootstrap guard in index.html.
 */
class RootErrorBoundary extends React.Component<RootErrorBoundaryProps, RootErrorBoundaryState> {
  constructor(props: RootErrorBoundaryProps) {
    super(props);
    this.state = { error: null };
  }

  static getDerivedStateFromError(error: Error): RootErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo): void {
    // Keep the component stack in the console; the panel only shows the summary.
    console.error('[cric-flow] render error', error, errorInfo.componentStack);
  }

  private handleReload = (): void => {
    window.location.reload();
  };

  render(): React.ReactNode {
    const { error } = this.state;
    if (!error) {
      return this.props.children;
    }

    const diagnosis = describeStartupError(error.message);

    return (
      <Box sx={{ p: 4, display: 'flex', justifyContent: 'center' }}>
        <Paper sx={{ p: 3, maxWidth: 820, width: '100%' }}>
          <Stack spacing={2}>
            <Typography variant="h6" color="error">
              The interface failed to render
            </Typography>
            <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
              {error.message}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              {diagnosis.hint}
            </Typography>
            {diagnosis.command ? (
              <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                {diagnosis.command}
              </Typography>
            ) : null}
            <Box>
              <Button variant="contained" onClick={this.handleReload}>
                Reload
              </Button>
            </Box>
          </Stack>
        </Paper>
      </Box>
    );
  }
}

export default RootErrorBoundary;
