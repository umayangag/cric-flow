import React, { useMemo, useState } from 'react';
import type { OpsStatus } from './OpsStatusTab';
import Box from '@mui/material/Box';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import PipelineStepDialog from './PipelineStepDialog';
import {
  derivePipelineSteps,
  statusIcon,
  getStatusColor,
} from '../utils/pipelineSteps';
import type { PipelineStep, StepStatus } from '../utils/pipelineSteps';

// Re-export types so existing consumers don't break
export type { PipelineStepId, StepStatus, PipelineStep } from '../utils/pipelineSteps';

type OpsPipelineGraphProps = {
  data: OpsStatus | null;
  onRefresh?: () => void;
};

const OpsPipelineGraph: React.FC<OpsPipelineGraphProps> = ({ data, onRefresh }) => {
  const [dialogStep, setDialogStep] = useState<PipelineStep | null>(null);
  const steps = useMemo(() => derivePipelineSteps(data), [data]);

  return (
    <>
      <Box
        sx={{
          display: 'flex',
          flexWrap: 'wrap',
          alignItems: 'center',
          gap: 0.5,
          py: 1,
        }}
      >
        {steps.map((step, index) => (
          <React.Fragment key={step.id}>
            {index > 0 && (
              <Typography
                component="span"
                sx={{ color: 'text.secondary', fontSize: 18, lineHeight: 1 }}
                aria-hidden
              >
                →
              </Typography>
            )}
            <Paper
              component="button"
              type="button"
              elevation={1}
              onClick={() => setDialogStep(step)}
              title={
                !step.runnable && step.status !== 'running'
                  ? 'Complete the previous step first.'
                  : undefined
              }
              sx={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 1,
                px: 1.5,
                py: 1,
                cursor: 'pointer',
                border: '1px solid',
                borderColor: step.status === 'pending' ? 'divider' : getStatusColor(step.status),
                borderRadius: 1,
                bgcolor:
                  step.status === 'success'
                    ? 'action.selected'
                    : step.status === 'running'
                      ? 'primary.50'
                      : 'background.paper',
                transition: 'border-color .15s, box-shadow .15s',
                ...(!step.runnable &&
                  step.status !== 'running' && {
                    opacity: 0.65,
                    cursor: 'default',
                  }),
                '&:hover': {
                  ...(step.runnable || step.status === 'running'
                    ? { borderColor: 'primary.main', boxShadow: 1 }
                    : {}),
                },
                '&:focus-visible': {
                  outline: '2px solid',
                  outlineColor: 'primary.main',
                  outlineOffset: 2,
                },
              }}
            >
              <Typography
                component="span"
                sx={{
                  fontSize: 16,
                  fontWeight: 600,
                  color: getStatusColor(step.status),
                  lineHeight: 1,
                  display: 'inline-flex',
                  alignItems: 'center',
                }}
                title={
                  step.status === 'optional'
                    ? 'Optional step'
                    : step.status === 'running'
                      ? 'Running'
                      : step.status
                }
              >
                {step.status === 'running' ? (
                  <CircularProgress size={16} color="inherit" sx={{ display: 'block' }} />
                ) : (
                  statusIcon[step.status as Exclude<StepStatus, 'running'>]
                )}
              </Typography>
              <Typography component="span" variant="body2" fontWeight={500}>
                {step.label}
              </Typography>
            </Paper>
          </React.Fragment>
        ))}
      </Box>

      <PipelineStepDialog
        step={dialogStep}
        onClose={() => setDialogStep(null)}
        onRefresh={onRefresh}
      />
    </>
  );
};

export default OpsPipelineGraph;
