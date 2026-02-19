import React, { useMemo, useState } from 'react';
import type { OpsStatus } from './OpsStatusTab';
import { api } from '../api';
import Box from '@mui/material/Box';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';

export type PipelineStepId =
  | 'import'
  | 'precompute'
  | 'export'
  | 'train_batting'
  | 'train_bowling'
  | 'train_fielding'
  | 'train_extras'
  | 'train_win'
  | 'auto_tune';

export type StepStatus = 'success' | 'stale' | 'pending' | 'error' | 'optional' | 'running';

export type PipelineStep = {
  id: PipelineStepId;
  label: string;
  status: StepStatus;
  command: string;
  description: string;
  /** Only runnable when previous step completed successfully (from backend). */
  runnable: boolean;
};

function asObj(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' ? (v as Record<string, unknown>) : {};
}

function getFormats(section: unknown): Record<string, unknown> {
  const obj = asObj(section);
  return asObj(obj.formats);
}

function derivePipelineSteps(data: OpsStatus | null): PipelineStep[] {
  const steps: PipelineStep[] = [
    {
      id: 'import',
      label: 'Import',
      status: 'pending',
      command: 'make migrate && make cricsheet-import',
      description: 'Apply migrations and import Cricsheet JSON into the database.',
      runnable: true,
    },
    {
      id: 'precompute',
      label: 'Precompute',
      status: 'pending',
      command: 'make precompute-all-all-formats',
      description: 'Compute form, consistency, and sequence features for each format (TEST, ODI, T20, T20I).',
      runnable: true,
    },
    {
      id: 'export',
      label: 'Export',
      status: 'pending',
      command: 'make export-dataset',
      description: 'Export unified (all-formats) and per-format batting/bowling CSVs to output/go-app when split_by_format is on.',
      runnable: true,
    },
    {
      id: 'train_batting',
      label: 'Train Batting',
      status: 'pending',
      command: 'make train-batting',
      description: 'Train unified (overall) plus per-format batting models (TEST, ODI, T20, T20I) from exported CSVs.',
      runnable: true,
    },
    {
      id: 'train_bowling',
      label: 'Train Bowling',
      status: 'pending',
      command: 'make train-bowling',
      description: 'Train unified (overall) plus per-format bowling models (TEST, ODI, T20, T20I) from exported CSVs.',
      runnable: true,
    },
    {
      id: 'train_fielding',
      label: 'Train Fielding',
      status: 'pending',
      command: 'make train-fielding CUTOFF=2025-01-01T00:00:00Z',
      description: 'Train unified (overall) plus per-format fielding models (set CUTOFF and GO_APP_URL as needed).',
      runnable: true,
    },
    {
      id: 'train_extras',
      label: 'Train Extras',
      status: 'pending',
      command: 'make train-extras CUTOFF=2025-01-01T00:00:00Z',
      description: 'Train unified (overall) plus per-format extras models (set CUTOFF and GO_APP_URL as needed).',
      runnable: true,
    },
    {
      id: 'train_win',
      label: 'Train Win',
      status: 'pending',
      command: 'make train-win CUTOFF=2025-01-01T00:00:00Z',
      description: 'Train unified (overall) plus per-format win prediction models (set CUTOFF and GO_APP_URL as needed).',
      runnable: true,
    },
    {
      id: 'auto_tune',
      label: 'Auto-tune',
      status: 'optional',
      command: 'make ml-auto-tune MODEL=all ALL_FORMATS=1',
      description: 'Optional: tune all models (batting, bowling, fielding, extras, win) per format and save params to DB. Set GO_APP_URL (and CUTOFF if needed) so params are stored.',
      runnable: true,
    },
  ];

  if (!data) return steps;

  const db = asObj(data.db);
  const counts = asObj(db.counts);
  const matchesCount = typeof counts.matches === 'number' ? counts.matches : 0;
  const importDone = data.services?.api_readiness === true && matchesCount > 0;

  const precomputeFormats = getFormats(data.precompute);
  let precomputeStatus: StepStatus = 'pending';
  for (const k of Object.keys(precomputeFormats)) {
    const fmt = asObj(precomputeFormats[k]);
    const s = fmt.status as string | undefined;
    if (s === 'ok') {
      precomputeStatus = 'success';
      break;
    }
    if (s === 'stale') precomputeStatus = 'stale';
  }
  if (precomputeStatus === 'pending' && Object.keys(precomputeFormats).length > 0)
    precomputeStatus = 'stale';

  const exportFormats = getFormats(data.exports);
  let exportDone = false;
  for (const k of Object.keys(exportFormats)) {
    const fmt = asObj(exportFormats[k]);
    const files = Array.isArray(fmt.files) ? fmt.files : [];
    if (files.some((f: unknown) => asObj(f).exists === true)) {
      exportDone = true;
      break;
    }
  }

  const artifactFormats = getFormats(data.artifacts);
  let battingDone = false;
  let bowlingDone = false;
  let fieldingDone = false;
  for (const k of Object.keys(artifactFormats)) {
    const fmt = asObj(artifactFormats[k]);
    const bat = asObj(fmt.batting);
    const bowl = asObj(fmt.bowling);
    const field = asObj(fmt.fielding);
    if (bat.loaded === true || bat.exists === true) battingDone = true;
    if (bowl.loaded === true || bowl.exists === true) bowlingDone = true;
    if (field.loaded === true || field.exists === true) fieldingDone = true;
  }

  steps[0].status = importDone ? 'success' : 'pending';
  steps[1].status = precomputeStatus;
  steps[2].status = exportDone ? 'success' : 'pending';
  steps[3].status = battingDone ? 'success' : 'pending';
  steps[4].status = bowlingDone ? 'success' : 'pending';
  steps[5].status = fieldingDone ? 'success' : 'pending';
  // train_extras (6) and train_win (7): no artifact check yet; use backend running/runnable only
  // steps[6], steps[7] stay pending unless running
  // Override with running and runnable from backend (next step only runnable after previous completed)
  const pipelineSteps = asObj(asObj(data.pipeline).steps);
  for (let i = 0; i < steps.length; i++) {
    const step = steps[i];
    const stepData = asObj(pipelineSteps[step.id]);
    const running = stepData.running === true;
    if (running) steps[i].status = 'running';
    // Default true when backend omits runnable (e.g. older API)
    steps[i].runnable = stepData.runnable !== false;
  }
  return steps;
}

const statusIcon: Record<Exclude<StepStatus, 'running'>, string> = {
  success: '✓',
  stale: '◐',
  pending: '○',
  error: '✗',
  optional: '◇',
};

type OpsPipelineGraphProps = {
  data: OpsStatus | null;
  onRefresh?: () => void;
};

const OpsPipelineGraph: React.FC<OpsPipelineGraphProps> = ({ data, onRefresh }) => {
  const [dialogStep, setDialogStep] = useState<PipelineStep | null>(null);
  const [copied, setCopied] = useState(false);
  const [runState, setRunState] = useState<
    'idle' | 'loading' | 'started' | 'run_from_root' | 'error'
  >('idle');
  const [runMessage, setRunMessage] = useState<string>('');
  const [runCommand, setRunCommand] = useState<string>('');

  const steps = useMemo(() => derivePipelineSteps(data), [data]);

  const handleRun = async (step: PipelineStep) => {
    setRunState('loading');
    setRunMessage('');
    setRunCommand('');
    try {
      const { status, data: res } = await api.opsPipelineRun(step.id);
      if (status === 202) {
        setRunState('started');
        setRunMessage('Step started. Status will update on refresh.');
        onRefresh?.();
      } else if (status === 501) {
        setRunState('run_from_root');
        setRunMessage(res.error || 'Run from project root');
        setRunCommand(res.command || step.command);
      } else {
        setRunState('error');
        setRunMessage(res.error || `HTTP ${status}`);
      }
    } catch (e) {
      setRunState('error');
      setRunMessage(e instanceof Error ? e.message : 'Request failed');
    }
  };

  const handleCopy = async (step: PipelineStep) => {
    try {
      await navigator.clipboard.writeText(step.command);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  };

  const getStatusColor = (status: StepStatus): string => {
    switch (status) {
      case 'success':
        return 'success.main';
      case 'stale':
        return 'warning.main';
      case 'pending':
        return 'text.secondary';
      case 'error':
        return 'error.main';
      case 'optional':
        return 'info.main';
      case 'running':
        return 'primary.main';
      default:
        return 'text.secondary';
    }
  };

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

      <Dialog
        open={dialogStep !== null}
        onClose={() => {
          setDialogStep(null);
          setRunState('idle');
          setRunMessage('');
          setRunCommand('');
        }}
        maxWidth="sm"
        fullWidth
      >
        {dialogStep && (
          <>
            <DialogTitle>{dialogStep.label}</DialogTitle>
            <DialogContent>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                {dialogStep.description}
              </Typography>
              {!dialogStep.runnable && dialogStep.status !== 'running' && (
                <Typography
                  variant="body2"
                  sx={{ mb: 1.5, p: 1, borderRadius: 1, bgcolor: 'action.hover' }}
                >
                  Complete the previous step first.
                </Typography>
              )}
              {runMessage && (
                <Typography
                  variant="body2"
                  sx={{
                    mb: 1.5,
                    p: 1,
                    borderRadius: 1,
                    bgcolor:
                      runState === 'error'
                        ? 'error.light'
                        : runState === 'run_from_root'
                          ? 'warning.light'
                          : 'action.selected',
                  }}
                >
                  {runMessage}
                </Typography>
              )}
              {(runState === 'run_from_root' && runCommand) || runState === 'idle' ? (
                <Box
                  component="pre"
                  sx={{
                    m: 0,
                    p: 1.5,
                    bgcolor: 'grey.100',
                    color: 'text.primary',
                    borderRadius: 1,
                    fontFamily: 'ui-monospace, Menlo, monospace',
                    fontSize: 13,
                    overflowX: 'auto',
                    border: '1px solid',
                    borderColor: 'divider',
                  }}
                >
                  {runState === 'run_from_root' && runCommand ? runCommand : dialogStep.command}
                </Box>
              ) : null}
            </DialogContent>
            <DialogActions>
              <Button
                onClick={() => {
                  setDialogStep(null);
                  setRunState('idle');
                  setRunMessage('');
                  setRunCommand('');
                }}
              >
                Close
              </Button>
              <Button
                variant="outlined"
                onClick={() =>
                  handleCopy({ ...dialogStep, command: runCommand || dialogStep.command })
                }
              >
                {copied ? 'Copied!' : 'Copy command'}
              </Button>
              <Button
                variant="contained"
                onClick={() => handleRun(dialogStep)}
                disabled={runState === 'loading' || !dialogStep.runnable}
              >
                {runState === 'loading' ? 'Running…' : 'Run'}
              </Button>
            </DialogActions>
          </>
        )}
      </Dialog>
    </>
  );
};

export default OpsPipelineGraph;
