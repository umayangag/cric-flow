import React, { useCallback, useState } from 'react';
import { Box, Button, Typography } from '@mui/material';
import { api } from '../api';
import type { PipelineProgressPayload } from '../types';
import { usePipelineProgressStream } from '../hooks/usePipelineProgressStream';
import PipelineStepProgressCard from './PipelineStepProgressCard';

type PipelineProgressPanelProps = {
  pipelineRunning: boolean;
  onRefresh?: () => void;
};

/**
 * Live pipeline progress, shown below the pipeline graph while anything is running.
 *
 * Renders one card per in-flight step. Steps can overlap — acquisition runs in its own
 * lane, and the run-plan executor drives several — so this reads the whole `steps`
 * list rather than assuming a single running step.
 */
const PipelineProgressPanel: React.FC<PipelineProgressPanelProps> = ({
  pipelineRunning,
  onRefresh,
}) => {
  const [stopLoading, setStopLoading] = useState(false);
  const [stopError, setStopError] = useState<string | null>(null);

  const handleRunCompleted = useCallback(
    (payload: PipelineProgressPayload) => {
      onRefresh?.();
      // Tell other tabs (e.g. ML Model Stats) a step finished so they can refresh
      // model-dependent views the user has open.
      try {
        if (typeof window !== 'undefined' && typeof window.dispatchEvent === 'function') {
          window.dispatchEvent(
            new CustomEvent<PipelineProgressPayload>('cric:pipeline-completed', {
              detail: payload,
            }),
          );
        }
      } catch {
        // Ignore environments without window / CustomEvent
      }
    },
    [onRefresh],
  );

  const { payload, error, reconnecting, connecting } = usePipelineProgressStream(
    pipelineRunning,
    handleRunCompleted,
  );

  const handleStopPipeline = useCallback(async () => {
    setStopError(null);
    setStopLoading(true);
    try {
      const { status, data: res } = await api.opsPipelineStop();
      if (status === 200) {
        onRefresh?.();
      } else {
        // A partial stop still cancelled the run here, so the panel refreshes as well as
        // reporting it: leaving the old state on screen beside "could not confirm" would
        // be a second thing for the operator to disbelieve (D-10).
        if (res.status === 'partially_cancelled') onRefresh?.();
        setStopError(res.error || `Failed (${status})`);
      }
    } catch (e) {
      setStopError(e instanceof Error ? e.message : 'Request failed');
    } finally {
      setStopLoading(false);
    }
  }, [onRefresh]);

  const runningSteps = payload?.running ? (payload.steps ?? []) : [];
  const showRunning = runningSteps.length > 0;

  if (!pipelineRunning && !showRunning && !error) {
    return null;
  }

  const stopButton = (
    <Button
      size="small"
      color="error"
      variant="outlined"
      disabled={stopLoading}
      onClick={handleStopPipeline}
    >
      {stopLoading ? 'Stopping…' : 'Stop pipeline'}
    </Button>
  );

  return (
    <Box
      sx={{
        mt: 2,
        p: 2,
        borderRadius: 1,
        bgcolor: showRunning ? 'action.hover' : 'grey.50',
        border: '1px solid',
        borderColor: 'divider',
      }}
    >
      <Typography variant="subtitle2" color="text.secondary" gutterBottom>
        Live progress
      </Typography>

      {error && (
        <Typography variant="body2" color="error">
          {error}
        </Typography>
      )}

      {!error && connecting && (
        <Box sx={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 2 }}>
          <Typography variant="body2" color="text.secondary">
            Connecting to live progress…
          </Typography>
          {stopButton}
        </Box>
      )}

      {!error && !connecting && !showRunning && (
        <Typography variant="body2" color="text.secondary">
          No pipeline in progress.
        </Typography>
      )}

      {!error && showRunning && (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
          {reconnecting && (
            <Typography variant="caption" color="text.secondary" sx={{ fontStyle: 'italic' }}>
              Reconnecting… (showing last status)
            </Typography>
          )}

          {runningSteps.map((step, index) => (
            <PipelineStepProgressCard key={step.step_id ?? index} step={step} />
          ))}

          <Box sx={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 2 }}>
            {runningSteps.length > 1 && (
              <Typography variant="body2" color="text.secondary">
                {runningSteps.length} steps running
              </Typography>
            )}
            {stopButton}
          </Box>

          {stopError && (
            <Typography variant="caption" color="error">
              {stopError}
            </Typography>
          )}
        </Box>
      )}
    </Box>
  );
};

export default PipelineProgressPanel;
