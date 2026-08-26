import React, { useCallback, useEffect, useState } from 'react';
import type { MLModelStat, ModelStatsResponse } from '../types';
import { api } from '../api';
import { Box } from '@mui/material';
import Typography from '@mui/material/Typography';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Button from '@mui/material/Button';
import AutoTuneForm from './AutoTuneForm';
import type { PipelineStep } from '../utils/pipelineSteps';

const DEFAULT_ALGORITHMS = ['rf', 'gb', 'quantile'];

export interface PipelineStepDialogProps {
  step: PipelineStep | null;
  onClose: () => void;
  onRefresh?: () => void;
}

const PipelineStepDialog: React.FC<PipelineStepDialogProps> = ({ step, onClose, onRefresh }) => {
  const [copied, setCopied] = useState(false);
  const [runState, setRunState] = useState<
    'idle' | 'loading' | 'started' | 'run_from_root' | 'error' | 'requires_confirmation'
  >('idle');
  const [runMessage, setRunMessage] = useState('');
  const [runHint, setRunHint] = useState('');
  const [runCommand, setRunCommand] = useState('');
  const [autoTuneModel, setAutoTuneModel] = useState('all');
  const [autoTuneFormat, setAutoTuneFormat] = useState('unified');
  const [autoTuneRescreen, setAutoTuneRescreen] = useState(false);
  const [autoTuneCutoff, setAutoTuneCutoff] = useState('');
  const [autoTuneAlgorithms, setAutoTuneAlgorithms] = useState<Set<string>>(
    () => new Set(DEFAULT_ALGORITHMS),
  );

  const modelNameForLookup = (m: string) =>
    m === 'all' ? '' : m.charAt(0).toUpperCase() + m.slice(1);
  const formatForLookup = (f: string) => (f === 'unified' ? 'Unified' : f || '');

  const loadDefaultAlgorithms = useCallback(async () => {
    const modelName = modelNameForLookup(autoTuneModel);
    const fmt = formatForLookup(autoTuneFormat);
    if (!modelName) {
      setAutoTuneAlgorithms(new Set(DEFAULT_ALGORITHMS));
      return;
    }
    try {
      const res = await api.getModelStats();
      const payload = res as unknown as ModelStatsResponse;
      const models = payload?.models ?? [];
      const match = models.find((m: MLModelStat) => {
        if (m.model_name !== modelName) return false;
        if (fmt) return m.match_format === fmt;
        return true;
      });
      setAutoTuneAlgorithms(
        match?.algorithms_requested?.length
          ? new Set(match.algorithms_requested)
          : new Set(DEFAULT_ALGORITHMS),
      );
    } catch {
      setAutoTuneAlgorithms(new Set(DEFAULT_ALGORITHMS));
    }
  }, [autoTuneModel, autoTuneFormat]);

  useEffect(() => {
    if (step?.id === 'auto_tune') loadDefaultAlgorithms();
  }, [step?.id, autoTuneModel, autoTuneFormat, loadDefaultAlgorithms]);

  const buildRunParams = (s: PipelineStep, extra?: Record<string, string>) =>
    s.id === 'auto_tune'
      ? {
          model: autoTuneModel,
          ...(autoTuneFormat === 'unified'
            ? { unified: '1' }
            : autoTuneFormat === ''
              ? { all_formats: '1' }
              : { format: autoTuneFormat }),
          ...(autoTuneRescreen ? { rescreen: '1' } : {}),
          ...(autoTuneCutoff.trim() ? { cutoff: autoTuneCutoff.trim() } : {}),
          ...(autoTuneAlgorithms.size > 0
            ? { algorithms: [...autoTuneAlgorithms].sort().join(',') }
            : {}),
          ...extra,
        }
      : { ...extra };

  const handleRun = async (s: PipelineStep, confirmUseDefault = false) => {
    setRunState('loading');
    setRunMessage('');
    setRunHint('');
    setRunCommand('');
    try {
      const params = confirmUseDefault
        ? { ...buildRunParams(s), confirm_use_default: '1' }
        : buildRunParams(s);
      const effectiveParams = Object.keys(params).length ? params : undefined;
      const { status, data: res } = await api.opsPipelineRun(s.id, effectiveParams);
      if (status === 202) {
        setRunState('started');
        setRunMessage('Step started. Status will update on refresh.');
        onRefresh?.();
      } else if (status === 501) {
        setRunState('run_from_root');
        setRunMessage(res.error || 'Run from project root');
        setRunCommand(res.command || s.command);
      } else if (status === 200 && res.requires_confirmation) {
        setRunState('requires_confirmation');
        setRunMessage(res.message || 'No auto-tuned parameters found. Train with default config?');
      } else {
        // The backend answers preconditions with {code, message, hint} so the UI can
        // name the next action instead of showing a bare red toast.
        setRunState('error');
        setRunMessage(res.message || res.error || `HTTP ${status}`);
        setRunHint(res.hint || '');
      }
    } catch (e) {
      setRunState('error');
      setRunMessage(e instanceof Error ? e.message : 'Request failed');
    }
  };

  const getAutoTuneCommand = () => {
    const parts = [`MODEL=${autoTuneModel}`];
    if (autoTuneFormat === 'unified') {
      /* no flag */
    } else if (autoTuneFormat === '') parts.push('ALL_FORMATS=1');
    else parts.push(`FORMAT=${autoTuneFormat}`);
    if (autoTuneRescreen) parts.push('RESCREEN=1');
    if (autoTuneCutoff.trim()) parts.push(`CUTOFF="${autoTuneCutoff.trim()}"`);
    if (autoTuneAlgorithms.size > 0)
      parts.push(`ALGORITHMS="${[...autoTuneAlgorithms].sort().join(',')}"`);
    return `make ml-auto-tune ${parts.join(' ')}`;
  };

  const handleCopy = async (s: PipelineStep) => {
    try {
      await navigator.clipboard.writeText(
        s.id === 'auto_tune' ? getAutoTuneCommand() : runCommand || s.command,
      );
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  };

  const handleClose = () => {
    setRunState('idle');
    setRunMessage('');
    setRunHint('');
    setRunCommand('');
    setAutoTuneRescreen(false);
    setAutoTuneCutoff('');
    setAutoTuneAlgorithms(new Set(DEFAULT_ALGORITHMS));
    onClose();
  };

  return (
    <Dialog open={step !== null} onClose={handleClose} maxWidth="sm" fullWidth>
      {step && (
        <>
          <DialogTitle>{step.label}</DialogTitle>
          <DialogContent>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
              {step.description}
            </Typography>
            {step.id === 'auto_tune' && (
              <AutoTuneForm
                model={autoTuneModel}
                onModelChange={setAutoTuneModel}
                format={autoTuneFormat}
                onFormatChange={setAutoTuneFormat}
                rescreen={autoTuneRescreen}
                onRescreenChange={setAutoTuneRescreen}
                cutoff={autoTuneCutoff}
                onCutoffChange={setAutoTuneCutoff}
                algorithms={autoTuneAlgorithms}
                onAlgorithmsChange={setAutoTuneAlgorithms}
              />
            )}
            {step.prerequisite && (
              <Typography
                variant="body2"
                sx={{ mb: 1.5, p: 1, borderRadius: 1, bgcolor: 'info.light' }}
              >
                {step.prerequisite}
              </Typography>
            )}
            {!step.runnable && step.status !== 'running' && (
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
                      : runState === 'run_from_root' || runState === 'requires_confirmation'
                        ? 'warning.light'
                        : 'action.selected',
                }}
              >
                {runMessage}
                {runHint && (
                  <Typography component="span" variant="body2" sx={{ display: 'block', mt: 0.5 }}>
                    {runHint}
                  </Typography>
                )}
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
                {runState === 'run_from_root' && runCommand
                  ? runCommand
                  : step.id === 'auto_tune'
                    ? getAutoTuneCommand()
                    : step.command}
              </Box>
            ) : null}
          </DialogContent>
          <DialogActions>
            <Button onClick={handleClose}>Close</Button>
            {runState === 'requires_confirmation' ? (
              <>
                <Button
                  variant="outlined"
                  onClick={() => {
                    setRunState('idle');
                    setRunMessage('');
                  }}
                >
                  Cancel
                </Button>
                <Button variant="contained" color="warning" onClick={() => handleRun(step, true)}>
                  Train with defaults
                </Button>
              </>
            ) : (
              <>
                <Button
                  variant="outlined"
                  onClick={() => handleCopy({ ...step, command: runCommand || step.command })}
                >
                  {copied ? 'Copied!' : 'Copy command'}
                </Button>
                <Button
                  variant="contained"
                  onClick={() => handleRun(step)}
                  disabled={runState === 'loading' || !step.runnable}
                >
                  {runState === 'loading' ? 'Running…' : 'Run'}
                </Button>
              </>
            )}
          </DialogActions>
        </>
      )}
    </Dialog>
  );
};

export default PipelineStepDialog;
