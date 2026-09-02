import React from 'react';
import { Box, LinearProgress, Typography } from '@mui/material';
import type { PipelineStepProgress } from '../types';
import {
  formatBytes,
  formatCount,
  formatDuration,
  formatMetricValue,
  formatRate,
} from '../utils/format';

/** Parameters the step was started with, as "model: all · cutoff: 2025-01-01". */
const StepParams: React.FC<{ params: Record<string, unknown> }> = ({ params }) => (
  <Typography variant="caption" color="text.secondary" component="div">
    {Object.entries(params)
      .map(([key, value]) => {
        const label = key.replace(/_/g, ' ');
        const val =
          typeof value === 'object' && value !== null && !Array.isArray(value)
            ? JSON.stringify(value)
            : String(value);
        return `${label}: ${val}`;
      })
      .join(' · ')}
  </Typography>
);

/** Download progress with a determinate bar when the server declared a size. */
const FetchDetails: React.FC<{ fetch: NonNullable<PipelineStepProgress['fetch']> }> = ({
  fetch,
}) => {
  const downloaded = fetch.downloaded_bytes ?? 0;
  const total = fetch.total_bytes ?? 0;
  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.25 }}>
        <Typography variant="caption" color="text.secondary">
          {formatBytes(downloaded)}
          {total > 0 ? ` of ${formatBytes(total)}` : ''} · {formatRate(fetch.bytes_per_sec)}
        </Typography>
        {total > 0 && (
          <Typography variant="caption" color="text.secondary">
            {Math.floor((downloaded / total) * 100)}%
          </Typography>
        )}
      </Box>
      {/* No Content-Length means no percentage to claim, so the bar says "working"
          rather than inventing a fraction. */}
      <LinearProgress
        variant={total > 0 ? 'determinate' : 'indeterminate'}
        value={total > 0 ? (downloaded / total) * 100 : undefined}
        sx={{ height: 6, borderRadius: 1 }}
      />
    </Box>
  );
};

/** Extraction progress by entry count. */
const ExtractDetails: React.FC<{ extract: NonNullable<PipelineStepProgress['extract']> }> = ({
  extract,
}) => {
  const entries = extract.entries ?? 0;
  const total = extract.entries_total ?? 0;
  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.25 }}>
        <Typography variant="caption" color="text.secondary">
          {formatCount(entries)}
          {total > 0 ? ` of ${formatCount(total)}` : ''} files · {formatBytes(extract.bytes ?? 0)}
        </Typography>
        {total > 0 && (
          <Typography variant="caption" color="text.secondary">
            {Math.floor((entries / total) * 100)}%
          </Typography>
        )}
      </Box>
      <LinearProgress
        variant={total > 0 ? 'determinate' : 'indeterminate'}
        value={total > 0 ? (entries / total) * 100 : undefined}
        sx={{ height: 6, borderRadius: 1 }}
      />
    </Box>
  );
};

const TRAINING_PHASE_LABELS: Record<string, string> = {
  load: 'Loading data',
  features: 'Feature selection',
  fit: 'Fitting',
  grid: 'Choosing hyperparameters',
  artifact: 'Writing artifacts',
  done: 'Finished',
};

/** Metrics as "rmse 24.1 · rows 12,345", with integers left unrounded. */
function formatMetrics(metrics: Record<string, number>): string {
  return Object.entries(metrics)
    .map(([key, value]) => {
      const label = key.replace(/_/g, ' ');
      return `${label} ${formatMetricValue(value)}`;
    })
    .join(' · ');
}

/** Milestones published by a trainer: phase, progress, metrics, dropped columns. */
const TrainingDetails: React.FC<{ training: NonNullable<PipelineStepProgress['training']> }> = ({
  training,
}) => {
  const current = training.current;
  const total = training.total;
  const determinate = typeof current === 'number' && typeof total === 'number' && total > 0;
  const dropped = training.dropped_columns ?? [];

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.75 }}>
      <Typography variant="caption" fontWeight={600} color="primary.main">
        {(training.phase && TRAINING_PHASE_LABELS[training.phase]) ?? training.phase ?? 'Running'}
        {training.format && ` · ${training.format}`}
        {determinate && ` · ${current} / ${total}`}
      </Typography>

      {training.message && (
        <Typography variant="caption" color="text.secondary">
          {training.message}
        </Typography>
      )}

      {training.metrics && Object.keys(training.metrics).length > 0 && (
        <Typography variant="caption" color="text.secondary">
          {formatMetrics(training.metrics)}
        </Typography>
      )}

      {/* The plan singles this out: a silently constant feature was being dropped
          with nobody told. Naming the columns is the entire point of surfacing it. */}
      {dropped.length > 0 && (
        <Typography variant="caption" color="warning.main">
          Dropped low-variance columns: {dropped.join(', ')}
          {training.dropped_columns_truncated
            ? ` (+${training.dropped_columns_truncated} more)`
            : ''}
        </Typography>
      )}

      {training.artifacts && training.artifacts.length > 0 && (
        <Typography variant="caption" color="text.secondary">
          Wrote {training.artifacts.map((a) => `${a.path} (${formatBytes(a.bytes)})`).join(', ')}
        </Typography>
      )}

      {determinate && (
        <LinearProgress
          variant="determinate"
          value={(current / total) * 100}
          sx={{ height: 6, borderRadius: 1 }}
        />
      )}
    </Box>
  );
};

/**
 * One running step. The panel renders one of these per in-flight step, so nothing
 * here assumes it is the only thing running.
 */
const PipelineStepProgressCard: React.FC<{ step: PipelineStepProgress }> = ({ step }) => (
  <Box
    sx={{
      display: 'flex',
      flexDirection: 'column',
      gap: 1,
      p: 1.5,
      borderRadius: 1,
      border: '1px solid',
      borderColor: 'divider',
      bgcolor: 'background.paper',
    }}
  >
    <Box sx={{ display: 'flex', flexWrap: 'wrap', alignItems: 'baseline', gap: 2 }}>
      <Typography variant="body2" fontWeight={600}>
        {step.step_label || step.step_id || 'Running'}
      </Typography>
      <Typography variant="body2" color="text.secondary">
        Elapsed: {formatDuration(step.elapsed_sec ?? 0)}
      </Typography>
      {step.estimated_remaining_sec != null && step.estimated_remaining_sec > 0 && (
        <Typography variant="body2" color="text.secondary">
          Est. remaining: ~{formatDuration(step.estimated_remaining_sec)}
        </Typography>
      )}
    </Box>

    {step.detail && (
      <Typography variant="body2" color="text.secondary">
        {step.detail}
      </Typography>
    )}
    {step.params && Object.keys(step.params).length > 0 && <StepParams params={step.params} />}

    {step.fetch && <FetchDetails fetch={step.fetch} />}
    {step.extract && <ExtractDetails extract={step.extract} />}

    {step.training && <TrainingDetails training={step.training} />}

    {/* Unreachable is not the same as nothing-yet. Both would otherwise be a blank
        panel, but only one of them is something the operator can fix. */}
    {step.progress_unavailable && (
      <Typography variant="caption" color="text.secondary" sx={{ fontStyle: 'italic' }}>
        Progress unavailable — ml-service could not be reached. The step is still running; only its
        detail is unknown.
      </Typography>
    )}

    {/* A data step that has started but not yet published a sample has unknown
        progress, and unknown is not zero: rendering 0 of 0 reads as a stall. */}
    {(step.step_id === 'fetch' || step.step_id === 'extract') && !step.fetch && !step.extract && (
      <Typography variant="caption" color="text.secondary" sx={{ fontStyle: 'italic' }}>
        Starting…
      </Typography>
    )}
  </Box>
);

export default PipelineStepProgressCard;
