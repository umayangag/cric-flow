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
import Checkbox from '@mui/material/Checkbox';
import FormControlLabel from '@mui/material/FormControlLabel';
import AutoTuneForm from './AutoTuneForm';
import type { PipelineStep } from '../utils/pipelineSteps';

const DEFAULT_ALGORITHMS = ['rf', 'gb', 'quantile'];

/** "fetch" -> "Fetch", so a backend step id reads as the start of a sentence. */
const titleCase = (value: string) => (value ? value.charAt(0).toUpperCase() + value.slice(1) : '');

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
  const [runSkipped, setRunSkipped] = useState<Record<string, string>>({});
  const [importRefresh, setImportRefresh] = useState(false);
  const [autoTuneModel, setAutoTuneModel] = useState('all');
  const [autoTuneFormat, setAutoTuneFormat] = useState('');
  const [autoTuneRescreen, setAutoTuneRescreen] = useState(false);
  const [autoTuneCutoff, setAutoTuneCutoff] = useState('');
  const [autoTuneAlgorithms, setAutoTuneAlgorithms] = useState<Set<string>>(
    () => new Set(DEFAULT_ALGORITHMS),
  );

  const modelNameForLookup = (m: string) =>
    m === 'all' ? '' : m.charAt(0).toUpperCase() + m.slice(1);
  const formatForLookup = (f: string) => f || '';

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

  const buildRunParams = (s: PipelineStep, extra?: Record<string, string>) => {
    if (s.id === 'auto_tune') {
      return {
        model: autoTuneModel,
        ...(autoTuneFormat === '' ? { all_formats: '1' } : { format: autoTuneFormat }),
        ...(autoTuneRescreen ? { rescreen: '1' } : {}),
        ...(autoTuneCutoff.trim() ? { cutoff: autoTuneCutoff.trim() } : {}),
        ...(autoTuneAlgorithms.size > 0
          ? { algorithms: [...autoTuneAlgorithms].sort().join(',') }
          : {}),
        ...extra,
      };
    }
    if (s.id === 'import') {
      return { ...(importRefresh ? { refresh: '1' } : {}), ...extra };
    }
    return { ...extra };
  };

  const handleRun = async (s: PipelineStep, confirmUseDefault = false) => {
    setRunState('loading');
    setRunMessage('');
    setRunHint('');
    setRunCommand('');
    setRunSkipped({});
    try {
      const params = confirmUseDefault
        ? { ...buildRunParams(s), confirm_use_default: '1' }
        : buildRunParams(s);
      const effectiveParams = Object.keys(params).length ? params : undefined;
      const { status, data: res } = await api.opsPipelineRun(s.id, effectiveParams);
      if (status === 202) {
        setRunState('started');
        setRunMessage('Step started. Status will update on refresh.');
        // A plan that skipped a step said why. Dropping that reason is how a
        // download that never ran comes to read as one that succeeded.
        setRunSkipped(res.skipped ?? {});
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
    if (autoTuneFormat === '') parts.push('ALL_FORMATS=1');
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
    setRunSkipped({});
    setImportRefresh(false);
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
            {step.id === 'import' && (
              <Box sx={{ mb: 1.5 }}>
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={importRefresh}
                      onChange={(e) => setImportRefresh(e.target.checked)}
                      size="small"
                    />
                  }
                  label="Re-download the archive even if this dataset is already on disk"
                />
                <Typography variant="caption" color="text.secondary" display="block">
                  Import skips the download when the dataset directory already holds the configured
                  archive, so a repeat run re-imports the same matches. Cricsheet republishes under
                  the same URL, so this is the only way to pick up newer ones — expect a transfer of
                  several hundred MB.
                </Typography>
              </Box>
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
            {Object.keys(runSkipped).length > 0 && (
              <Box sx={{ mb: 1.5, p: 1, borderRadius: 1, bgcolor: 'warning.light' }}>
                <Typography variant="body2">Not every step in the plan will run:</Typography>
                {Object.entries(runSkipped).map(([stepID, reason]) => (
                  <Typography key={stepID} variant="body2" sx={{ mt: 0.5 }}>
                    {`${titleCase(stepID)} skipped — ${reason}`}
                  </Typography>
                ))}
              </Box>
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
