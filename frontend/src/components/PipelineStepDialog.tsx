import React, { useState } from 'react';
import { api } from '../api';
import {
  Box,
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  TextField,
  Typography,
} from '@mui/material';
import type { PipelineStep } from '../utils/pipelineSteps';

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
    'idle' | 'loading' | 'started' | 'run_from_root' | 'error'
  >('idle');
  const [runMessage, setRunMessage] = useState('');
  const [runHint, setRunHint] = useState('');
  const [runCommand, setRunCommand] = useState('');
  const [runSkipped, setRunSkipped] = useState<Record<string, string>>({});
  const [importRefresh, setImportRefresh] = useState(false);
  const [cutoff, setCutoff] = useState('');
  const [runID, setRunID] = useState('');

  /** Steps that take a training cutoff. Reload takes a run id instead; import takes neither. */
  const takesCutoff = (id: PipelineStep['id']) => id === 'retrain' || id === 'evaluate';

  const buildRunParams = (s: PipelineStep, extra?: Record<string, string>) => {
    if (takesCutoff(s.id)) {
      return { ...(cutoff.trim() ? { cutoff: cutoff.trim() } : {}), ...extra };
    }
    if (s.id === 'reload') {
      return { ...(runID.trim() ? { run_id: runID.trim() } : {}), ...extra };
    }
    if (s.id === 'import') {
      return { ...(importRefresh ? { refresh: '1' } : {}), ...extra };
    }
    return { ...extra };
  };

  const handleRun = async (s: PipelineStep) => {
    setRunState('loading');
    setRunMessage('');
    setRunHint('');
    setRunCommand('');
    setRunSkipped({});
    try {
      const params = buildRunParams(s);
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

  /** The command as the operator's own inputs would make it, so Copy matches Run. */
  const commandFor = (s: PipelineStep) => {
    if (takesCutoff(s.id) && cutoff.trim()) return `make ${s.id} CUTOFF=${cutoff.trim()}`;
    if (s.id === 'reload' && runID.trim()) return `make reload RUN=${runID.trim()}`;
    return s.command;
  };

  const handleCopy = async (s: PipelineStep) => {
    try {
      await navigator.clipboard.writeText(runCommand || commandFor(s));
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
    setCutoff('');
    setRunID('');
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
            {takesCutoff(step.id) && (
              <Box sx={{ mb: 1.5 }}>
                <TextField
                  label="Cutoff (YYYY-MM-DD)"
                  value={cutoff}
                  onChange={(e) => setCutoff(e.target.value)}
                  size="small"
                  fullWidth
                />
                <Typography variant="caption" color="text.secondary" display="block">
                  Rows before the cutoff train the models; rows at or after it are the holdout the
                  run is scored on. Leave blank to use today.
                </Typography>
              </Box>
            )}
            {step.id === 'reload' && (
              <Box sx={{ mb: 1.5 }}>
                <TextField
                  label="Run id (optional)"
                  value={runID}
                  onChange={(e) => setRunID(e.target.value)}
                  size="small"
                  fullWidth
                />
                <Typography variant="caption" color="text.secondary" display="block">
                  Leave blank to load the newest run — the one a retrain just built. Name a run to
                  swap back to it — the Artifacts panel lists the ones on disk.
                </Typography>
              </Box>
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
                      : runState === 'run_from_root'
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
                {runState === 'run_from_root' && runCommand ? runCommand : commandFor(step)}
              </Box>
            ) : null}
          </DialogContent>
          <DialogActions>
            <Button onClick={handleClose}>Close</Button>
            <Button variant="outlined" onClick={() => handleCopy(step)}>
              {copied ? 'Copied!' : 'Copy command'}
            </Button>
            <Button
              variant="contained"
              onClick={() => handleRun(step)}
              disabled={runState === 'loading' || !step.runnable}
            >
              {runState === 'loading' ? 'Running…' : 'Run'}
            </Button>
          </DialogActions>
        </>
      )}
    </Dialog>
  );
};

export default PipelineStepDialog;
