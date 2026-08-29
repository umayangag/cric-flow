import React, { useCallback, useEffect, useState } from 'react';
import { Box } from '@mui/material';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import LinearProgress from '@mui/material/LinearProgress';
import MenuItem from '@mui/material/MenuItem';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import ErrorNotice from './common/ErrorNotice';
import { api } from '../api';
import type { RunPlanState, RunPlanStep, RunPlanStepStatus } from '../types';

/** How often to re-read plan state while one is running. */
const ACTIVE_POLL_MS = 3000;
/** And while nothing is, so a plan started elsewhere still shows up. */
const IDLE_POLL_MS = 15000;

const STATUS_COLOUR: Record<
  RunPlanStepStatus,
  'default' | 'info' | 'success' | 'error' | 'warning'
> = {
  PENDING: 'default',
  RUNNING: 'info',
  COMPLETED: 'success',
  FAILED: 'error',
  CANCELLED: 'warning',
  SKIPPED: 'default',
};

const PLAN_DESCRIPTIONS: Record<string, string> = {
  full: 'Import → precompute → export → train every model.',
  'retrain-only': 'Re-train the models against data already imported and exported.',
  'data-refresh': 'Re-import and re-derive without touching the models.',
  tune: 'Auto-tune, then re-train every model on the parameters it found. Run it when the feature space has changed and the saved parameters are no longer trustworthy.',
};

/**
 * The plans built around an optional step, so the caption does not tell the operator
 * something false.
 *
 * Every other plan leaves optional steps out by definition, and the caption used to
 * say so unconditionally — which stopped being true the moment `tune` existed, since
 * auto-tune is the step it exists to run.
 */
const PLANS_INCLUDING_OPTIONAL_STEPS = new Set(['tune']);

function planCaption(plan: string): string {
  const description = PLAN_DESCRIPTIONS[plan] ?? 'A named sequence of pipeline steps.';
  if (PLANS_INCLUDING_OPTIONAL_STEPS.has(plan)) return description;
  return `${description} Optional steps (auto-tune, combination-meta) are not included — run those yourself.`;
}

/** One step's row: where it got to, and why not when it failed. */
const PlanStepRow: React.FC<{ step: RunPlanStep }> = ({ step }) => {
  // go-app already formats an ml-service precondition as `CODE: message — hint`
  // (F-1's MLError), so the raw string is actionable as it stands. Splitting it into
  // code/message/hint is what OpsMigrationsTable does with the same string, via a
  // parser that arrives with O-5; this renders it whole rather than duplicating that
  // parser into a second place for them to drift apart in.
  const failed = step.status === 'FAILED' && Boolean(step.error);

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        gap: 0.5,
        py: 0.75,
        borderBottom: '1px solid',
        borderColor: 'divider',
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, flexWrap: 'wrap' }}>
        <Chip size="small" color={STATUS_COLOUR[step.status]} label={step.status} />
        <Typography variant="body2">{step.label}</Typography>
        {step.status === 'SKIPPED' && (
          <Typography variant="caption" color="text.secondary">
            {/*
              The backend's own reason, not a label chosen here. A step can be skipped
              because a resume already ran it or because the box already holds what it
              would produce, and rendering both as "already done" would hide which.
            */}
            {step.note ?? 'not repeated'}
          </Typography>
        )}
      </Box>
      {failed && (
        <Typography
          variant="caption"
          color="error"
          component="div"
          sx={{ pl: 1, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}
        >
          {step.error}
        </Typography>
      )}
    </Box>
  );
};

/**
 * Run the whole pipeline from one action, and watch it.
 *
 * `make up-all` and `make full-pipeline` have existed in the Makefile for as long as
 * the pipeline has; from this console the same thing was eleven clicks with waiting in
 * between. The state is the server's, not this component's — which is what lets the
 * panel show a plan it did not start, after a reload or from another tab.
 */
const RunPlanPanel: React.FC<{ onRefresh?: () => void }> = ({ onRefresh }) => {
  const [state, setState] = useState<RunPlanState | null>(null);
  const [plan, setPlan] = useState('full');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const next = await api.opsRunPlan({ signal });
      setState(next);
    } catch (e) {
      if ((e as { name?: string }).name === 'AbortError') return;
      // A failed poll is not worth an alert: the previous state is still the best
      // answer available, and the next tick usually fixes it.
      setState((prev) => prev);
    }
  }, []);

  useEffect(() => {
    const ac = new AbortController();
    void load(ac.signal);
    // Polled even when idle, so a plan started from another tab appears here without
    // the operator reloading to find out.
    const interval = setInterval(
      () => void load(ac.signal),
      state?.running ? ACTIVE_POLL_MS : IDLE_POLL_MS,
    );
    return () => {
      ac.abort();
      clearInterval(interval);
    };
  }, [load, state?.running]);

  const start = useCallback(
    async (resume: boolean) => {
      setError(null);
      setSubmitting(true);
      try {
        const { status, data } = await api.opsRunPlanStart(
          resume ? { plan, resume: true } : { plan },
        );
        if (status === 202) {
          await load();
          onRefresh?.();
          return;
        }
        setError(data.error || `Failed (${status})`);
      } catch (e) {
        setError(e instanceof Error ? e.message : 'Request failed');
      } finally {
        setSubmitting(false);
      }
    },
    [plan, load, onRefresh],
  );

  const steps = state?.steps ?? [];
  const finished = steps.filter((s) => s.status === 'COMPLETED' || s.status === 'SKIPPED').length;
  const running = state?.running ?? false;
  const canResume = Boolean(state?.resume_from) && !running;
  const plans = state?.plans ?? ['full'];

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5, mt: 2 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, flexWrap: 'wrap' }}>
        <TextField
          select
          size="small"
          label="Plan"
          value={plan}
          onChange={(e) => setPlan(e.target.value)}
          disabled={running}
          sx={{ minWidth: 180 }}
          SelectProps={{ MenuProps: { disableScrollLock: true } }}
        >
          {plans.map((name) => (
            <MenuItem key={name} value={name}>
              {name}
            </MenuItem>
          ))}
        </TextField>

        <Button
          variant="contained"
          size="small"
          disabled={running || submitting}
          onClick={() => void start(false)}
        >
          {running ? 'Running…' : 'Run pipeline'}
        </Button>

        {/* describeChild: without it MUI puts the title on aria-label and *replaces*
            the button's accessible name, so a screen reader announces the tooltip
            instead of "Resume from precompute". */}
        {canResume && (
          <Tooltip
            describeChild
            title={`Continues from ${state?.resume_from}, skipping what already completed.`}
          >
            <Button
              variant="outlined"
              size="small"
              color="warning"
              disabled={submitting}
              onClick={() => void start(true)}
            >
              Resume from {state?.resume_from}
            </Button>
          </Tooltip>
        )}
      </Box>

      <Typography variant="caption" color="text.secondary">
        {planCaption(plan)}
      </Typography>

      <ErrorNotice error={error} title="Run plan" />

      {steps.length > 0 && (
        <Box>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.5 }}>
            <Typography variant="body2" color="text.secondary">
              Plan <strong>{state?.plan}</strong>
              {/* A plan that is not running is history, and saying so stops a finished
                  run being mistaken for a stalled one. */}
              {!running && ' — not running'}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              {finished} / {steps.length}
            </Typography>
          </Box>
          <LinearProgress
            variant="determinate"
            value={(finished / steps.length) * 100}
            sx={{ height: 6, borderRadius: 1, mb: 1 }}
          />
          {steps.map((step) => (
            <PlanStepRow key={step.step_id} step={step} />
          ))}
        </Box>
      )}

      {state && steps.length === 0 && (
        <Typography variant="body2" color="text.secondary">
          No pipeline run yet. Running a plan chains the steps server-side, so closing this page
          does not stop it.
        </Typography>
      )}
    </Box>
  );
};

export default RunPlanPanel;
