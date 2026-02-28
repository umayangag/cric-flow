import React, { useCallback, useEffect, useMemo, useState } from 'react';
import type { OpsStatus } from './OpsStatusTab';
import type { MLModelStat, ModelStatsResponse } from '../types';
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
import Checkbox from '@mui/material/Checkbox';
import FormControl from '@mui/material/FormControl';
import FormControlLabel from '@mui/material/FormControlLabel';
import InputLabel from '@mui/material/InputLabel';
import MenuItem from '@mui/material/MenuItem';
import Select from '@mui/material/Select';
import TextField from '@mui/material/TextField';

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
      description:
        'Apply migrations and import Cricsheet JSON into the database. Run from project root.',
      runnable: true,
    },
    {
      id: 'precompute',
      label: 'Precompute',
      status: 'pending',
      command: 'make precompute-all-all-formats',
      description:
        'Compute form, consistency, and sequence features for all formats (TEST, ODI, T20, T20I). Run from project root.',
      runnable: true,
    },
    {
      id: 'export',
      label: 'Export',
      status: 'pending',
      command: 'make export-dataset',
      description:
        'Export unified and per-format batting/bowling CSVs to output/go-app (when export.split_by_format is on). Run from project root.',
      runnable: true,
    },
    {
      id: 'train_batting',
      label: 'Train Batting',
      status: 'pending',
      command: 'make train-batting',
      description:
        'Train per-format and unified (legacy) batting models from exported CSVs. Run from project root.',
      runnable: true,
    },
    {
      id: 'train_bowling',
      label: 'Train Bowling',
      status: 'pending',
      command: 'make train-bowling',
      description:
        'Train per-format and unified (legacy) bowling models from exported CSVs. Run from project root.',
      runnable: true,
    },
    {
      id: 'train_fielding',
      label: 'Train Fielding',
      status: 'pending',
      command: 'make train-fielding CUTOFF=2025-01-01T00:00:00Z',
      description:
        'Train unified plus per-format fielding models. Set CUTOFF (RFC3339) and GO_APP_URL; or use FIELDING_CSV=<path>. Run from project root.',
      runnable: true,
    },
    {
      id: 'train_extras',
      label: 'Train Extras',
      status: 'pending',
      command: 'make train-extras CUTOFF=2025-01-01T00:00:00Z',
      description:
        'Train unified plus per-format extras models. Set CUTOFF and GO_APP_URL; or EXTRAS_CSV=<path>. Run from project root.',
      runnable: true,
    },
    {
      id: 'train_win',
      label: 'Train Win',
      status: 'pending',
      command: 'make train-win CUTOFF=2025-01-01T00:00:00Z',
      description:
        'Train unified plus per-format win models. Set CUTOFF and GO_APP_URL; or WIN_CSV=<path>. Run from project root.',
      runnable: true,
    },
    {
      id: 'auto_tune',
      label: 'Auto-tune',
      status: 'optional',
      command: 'make ml-auto-tune MODEL=all ALL_FORMATS=1',
      description:
        'Optional: tune the unified model and all per-format models (batting, bowling, fielding, extras, win). Best params are saved to DB when GO_APP_URL is set. You can run this from the UI or from project root.',
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
  // train_extras (6) and train_win (7): no artifact check; use backend completed + running/runnable
  // Override with running, runnable, and completed from backend
  const pipelineSteps = asObj(asObj(data.pipeline).steps);
  for (let i = 0; i < steps.length; i++) {
    const step = steps[i];
    const stepData = asObj(pipelineSteps[step.id]);
    const running = stepData.running === true;
    const completed = stepData.completed === true;
    if (running) steps[i].status = 'running';
    else if (completed) steps[i].status = 'success';
    // Default true when backend omits runnable (e.g. older API)
    steps[i].runnable = stepData.runnable !== false;
  }
  // Import can always be retriggered to reset the pipeline; never grey it out
  const importStep = steps.find((s) => s.id === 'import');
  if (importStep) importStep.runnable = true;
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

const AUTO_TUNE_MODELS = [
  { value: 'all', label: 'All' },
  { value: 'batting', label: 'Batting' },
  { value: 'bowling', label: 'Bowling' },
  { value: 'fielding', label: 'Fielding' },
  { value: 'extras', label: 'Extras' },
  { value: 'win', label: 'Win' },
] as const;

const AUTO_TUNE_FORMATS = [
  { value: 'unified', label: 'Unified only' },
  { value: '', label: 'All formats' },
  { value: 'TEST', label: 'TEST' },
  { value: 'ODI', label: 'ODI' },
  { value: 'T20', label: 'T20' },
  { value: 'T20I', label: 'T20I' },
] as const;

const AUTO_TUNE_ALGORITHMS = [
  { value: 'rf', label: 'Random Forest' },
  { value: 'gb', label: 'Gradient Boosting' },
  { value: 'quantile', label: 'Quantile Regressor' },
  { value: 'et', label: 'Extra Trees' },
  { value: 'hgb', label: 'Hist Gradient Boosting' },
  { value: 'stacked', label: 'Stacking Regressor' },
  { value: 'mlp', label: 'MLP (Neural Network)' },
] as const;

const DEFAULT_ALGORITHMS = ['rf', 'gb', 'quantile'];

const OpsPipelineGraph: React.FC<OpsPipelineGraphProps> = ({ data, onRefresh }) => {
  const [dialogStep, setDialogStep] = useState<PipelineStep | null>(null);
  const [copied, setCopied] = useState(false);
  const [runState, setRunState] = useState<
    'idle' | 'loading' | 'started' | 'run_from_root' | 'error' | 'requires_confirmation'
  >('idle');
  const [runMessage, setRunMessage] = useState<string>('');
  const [runCommand, setRunCommand] = useState<string>('');
  const [autoTuneModel, setAutoTuneModel] = useState<string>('all');
  const [autoTuneFormat, setAutoTuneFormat] = useState<string>('unified');
  const [autoTuneRescreen, setAutoTuneRescreen] = useState<boolean>(false);
  const [autoTuneCutoff, setAutoTuneCutoff] = useState<string>('');
  const [autoTuneAlgorithms, setAutoTuneAlgorithms] = useState<Set<string>>(
    () => new Set(DEFAULT_ALGORITHMS)
  );

  const steps = useMemo(() => derivePipelineSteps(data), [data]);

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
      const res = await api.modelStats();
      const payload = res as unknown as ModelStatsResponse;
      const models = payload?.models ?? [];
      const match = models.find((m: MLModelStat) => {
        if (m.model_name !== modelName) return false;
        if (fmt) return m.match_format === fmt;
        return true;
      });
      if (match?.algorithms_requested?.length) {
        setAutoTuneAlgorithms(new Set(match.algorithms_requested));
      } else {
        setAutoTuneAlgorithms(new Set(DEFAULT_ALGORITHMS));
      }
    } catch {
      setAutoTuneAlgorithms(new Set(DEFAULT_ALGORITHMS));
    }
  }, [autoTuneModel, autoTuneFormat]);

  useEffect(() => {
    if (dialogStep?.id === 'auto_tune') {
      loadDefaultAlgorithms();
    }
  }, [dialogStep?.id, autoTuneModel, autoTuneFormat, loadDefaultAlgorithms]);

  const buildRunParams = (step: PipelineStep, extra?: Record<string, string>) =>
    step.id === 'auto_tune'
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

  const handleRun = async (step: PipelineStep, confirmUseDefault = false) => {
    setRunState('loading');
    setRunMessage('');
    setRunCommand('');
    try {
      const params = confirmUseDefault
        ? { ...buildRunParams(step), confirm_use_default: '1' }
        : buildRunParams(step);
      const effectiveParams = Object.keys(params).length ? params : undefined;
      const { status, data: res } = await api.opsPipelineRun(step.id, effectiveParams);
      if (status === 202) {
        setRunState('started');
        setRunMessage('Step started. Status will update on refresh.');
        onRefresh?.();
      } else if (status === 501) {
        setRunState('run_from_root');
        setRunMessage(res.error || 'Run from project root');
        setRunCommand(res.command || step.command);
      } else if (status === 200 && res.requires_confirmation) {
        setRunState('requires_confirmation');
        setRunMessage(res.message || 'No auto-tuned parameters found. Train with default config?');
      } else {
        setRunState('error');
        setRunMessage(res.error || `HTTP ${status}`);
      }
    } catch (e) {
      setRunState('error');
      setRunMessage(e instanceof Error ? e.message : 'Request failed');
    }
  };

  const handleRunWithDefaultConfirm = async (step: PipelineStep) => {
    await handleRun(step, true);
  };

  const getAutoTuneCommand = () => {
    const parts = [`MODEL=${autoTuneModel}`];
    if (autoTuneFormat === 'unified') {
      // no format flag
    } else if (autoTuneFormat === '') {
      parts.push('ALL_FORMATS=1');
    } else {
      parts.push(`FORMAT=${autoTuneFormat}`);
    }
    if (autoTuneRescreen) parts.push('RESCREEN=1');
    if (autoTuneCutoff.trim()) parts.push(`CUTOFF="${autoTuneCutoff.trim()}"`);
    if (autoTuneAlgorithms.size > 0) parts.push(`ALGORITHMS="${[...autoTuneAlgorithms].sort().join(',')}"`);
    return `make ml-auto-tune ${parts.join(' ')}`;
  };

  const handleCopy = async (step: PipelineStep) => {
    try {
      const text = step.id === 'auto_tune' ? getAutoTuneCommand() : runCommand || step.command;
      await navigator.clipboard.writeText(text);
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
              {dialogStep.id === 'auto_tune' && (
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, mb: 1.5 }}>
                  <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 2 }}>
                    <FormControl size="small" sx={{ minWidth: 140 }}>
                      <InputLabel id="autotune-model-label">Model</InputLabel>
                      <Select
                        labelId="autotune-model-label"
                        value={autoTuneModel}
                        label="Model"
                        onChange={(e) => setAutoTuneModel(e.target.value)}
                      >
                        {AUTO_TUNE_MODELS.map((o) => (
                          <MenuItem key={o.value} value={o.value}>
                            {o.label}
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>
                    <FormControl size="small" sx={{ minWidth: 140 }}>
                      <InputLabel id="autotune-format-label">Format</InputLabel>
                      <Select
                        labelId="autotune-format-label"
                        value={autoTuneFormat}
                        label="Format"
                        onChange={(e) => setAutoTuneFormat(e.target.value)}
                      >
                        {AUTO_TUNE_FORMATS.map((o) => (
                          <MenuItem key={o.value || 'all'} value={o.value}>
                            {o.label}
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>
                  </Box>
                  <FormControlLabel
                    control={
                      <Checkbox
                        checked={autoTuneRescreen}
                        onChange={(e) => setAutoTuneRescreen(e.target.checked)}
                        size="small"
                      />
                    }
                    label="Rescreen: start from scratch (full algorithm search, ignore prior tuning)"
                  />
                  <TextField
                    size="small"
                    label="Cutoff (optional, RFC3339)"
                    placeholder="2024-01-01T00:00:00Z"
                    value={autoTuneCutoff}
                    onChange={(e) => setAutoTuneCutoff(e.target.value)}
                    sx={{ maxWidth: 320 }}
                    helperText="Training data cutoff for API. Leave empty to use default."
                  />
                  <Box>
                    <Typography variant="caption" color="text.secondary" display="block" sx={{ mb: 0.5 }}>
                      Algorithms to consider (only selected will be used). Default: last used for this model+format.
                    </Typography>
                    <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
                      {AUTO_TUNE_ALGORITHMS.map(({ value, label }) => (
                        <FormControlLabel
                          key={value}
                          control={
                            <Checkbox
                              checked={autoTuneAlgorithms.has(value)}
                              onChange={(e) => {
                                const next = new Set(autoTuneAlgorithms);
                                if (e.target.checked) next.add(value);
                                else next.delete(value);
                                setAutoTuneAlgorithms(next);
                              }}
                              size="small"
                            />
                          }
                          label={label}
                        />
                      ))}
                    </Box>
                  </Box>
                </Box>
              )}
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
                        : runState === 'run_from_root' || runState === 'requires_confirmation'
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
                  {runState === 'run_from_root' && runCommand
                    ? runCommand
                    : dialogStep.id === 'auto_tune'
                      ? getAutoTuneCommand()
                      : dialogStep.command}
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
                  setAutoTuneRescreen(false);
                  setAutoTuneCutoff('');
                  setAutoTuneAlgorithms(new Set(DEFAULT_ALGORITHMS));
                }}
              >
                Close
              </Button>
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
                  <Button
                    variant="contained"
                    color="warning"
                    onClick={() => handleRunWithDefaultConfirm(dialogStep)}
                  >
                    Train with defaults
                  </Button>
                </>
              ) : (
                <>
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
                </>
              )}
            </DialogActions>
          </>
        )}
      </Dialog>
    </>
  );
};

export default OpsPipelineGraph;
