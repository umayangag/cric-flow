import React, { useCallback, useEffect, useRef, useState } from 'react';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import LinearProgress from '@mui/material/LinearProgress';
import Typography from '@mui/material/Typography';
import { api } from '../api';
import type { PipelineProgressPayload } from '../types';

const MAX_STREAM_RETRIES = 5;
const RETRY_DELAY_MS = 3000;
/** Keep showing last progress for this long after connection loss before showing error. */
const STALE_PROGRESS_BUFFER_MS = 15000;

function formatElapsed(sec: number): string {
  const m = Math.floor(sec / 60);
  const s = Math.floor(sec % 60);
  if (m >= 60) {
    const h = Math.floor(m / 60);
    return `${h}h ${m % 60}m`;
  }
  return m > 0 ? `${m}m ${s}s` : `${s}s`;
}

function formatActivity(activity: string): string {
  const labels: Record<string, string> = {
    loading_data: 'Loading data',
    screening: 'Screening algorithms',
    cross_validating: 'Cross-validating',
    screening_done: 'Screening complete',
    running_trial: 'Running Optuna trial',
    initializing: 'Initializing',
  };
  return labels[activity] ?? activity.replace(/_/g, ' ');
}

type PipelineProgressPanelProps = {
  pipelineRunning: boolean;
  onRefresh?: () => void;
};

/**
 * Live pipeline progress via SSE. Shown below the pipeline graph when a step is running.
 * Auto-retries the stream connection on failure (e.g. proxy timeout) up to MAX_STREAM_RETRIES.
 */
const PipelineProgressPanel: React.FC<PipelineProgressPanelProps> = ({
  pipelineRunning,
  onRefresh,
}) => {
  const [payload, setPayload] = useState<PipelineProgressPayload | null>(null);
  const [streamError, setStreamError] = useState<string | null>(null);
  const [reconnectingBuffered, setReconnectingBuffered] = useState(false);
  const [retryCount, setRetryCount] = useState(0);
  const [stopLoading, setStopLoading] = useState(false);
  const [stopError, setStopError] = useState<string | null>(null);
  const wasRunningRef = useRef(false);
  const onRefreshRef = useRef(onRefresh);
  const showErrorAfterBufferRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  onRefreshRef.current = onRefresh;

  const doRefresh = useCallback(() => {
    onRefreshRef.current?.();
  }, []);

  const handleStopPipeline = useCallback(async () => {
    setStopError(null);
    setStopLoading(true);
    try {
      const { status, data: res } = await api.opsPipelineStop();
      if (status === 200) {
        doRefresh();
      } else {
        setStopError(res.error || `Failed (${status})`);
      }
    } catch (e) {
      setStopError(e instanceof Error ? e.message : 'Request failed');
    } finally {
      setStopLoading(false);
    }
  }, [doRefresh]);

  useEffect(() => {
    if (!pipelineRunning) {
      setStreamError(null);
      setReconnectingBuffered(false);
      setRetryCount(0);
      setPayload((prev) => (prev?.running ? { ...prev, running: false } : prev));
      return;
    }
    setStreamError(null);
    setReconnectingBuffered(false);
    const ac = new AbortController();
    let mounted = true;
    let retryTimeout: ReturnType<typeof setTimeout> | null = null;

    const runStream = () => {
      api
        .subscribePipelineProgress(ac.signal, (p) => {
          if (mounted) {
            if (showErrorAfterBufferRef.current) {
              clearTimeout(showErrorAfterBufferRef.current);
              showErrorAfterBufferRef.current = null;
            }
            setStreamError(null);
            setReconnectingBuffered(false);
            setPayload(p);
            if (wasRunningRef.current && p.running === false) {
              wasRunningRef.current = false;
              doRefresh();
            } else if (p.running === true) {
              wasRunningRef.current = true;
            }
          }
        })
        .then(() => {
          if (mounted) doRefresh();
        })
        .catch((e) => {
          if (!mounted || (e as { name?: string }).name === 'AbortError') return;
          const message = e instanceof Error ? e.message : String(e);
          const isLastRetry = retryCount >= MAX_STREAM_RETRIES - 1;
          if (mounted) setReconnectingBuffered(true);
          // Keep showing last progress for STALE_PROGRESS_BUFFER_MS; only then show error.
          if (showErrorAfterBufferRef.current) clearTimeout(showErrorAfterBufferRef.current);
          showErrorAfterBufferRef.current = setTimeout(() => {
            showErrorAfterBufferRef.current = null;
            if (mounted) {
              setReconnectingBuffered(false);
              setStreamError(
                isLastRetry
                  ? `${message}. Click Refresh to try again.`
                  : 'Connection lost. Reconnecting…',
              );
            }
          }, STALE_PROGRESS_BUFFER_MS);
          if (!isLastRetry) {
            retryTimeout = setTimeout(() => {
              setRetryCount((c) => c + 1);
            }, RETRY_DELAY_MS);
          }
        });
    };

    runStream();

    return () => {
      mounted = false;
      if (retryTimeout) clearTimeout(retryTimeout);
      if (showErrorAfterBufferRef.current) {
        clearTimeout(showErrorAfterBufferRef.current);
        showErrorAfterBufferRef.current = null;
      }
      ac.abort();
    };
  }, [pipelineRunning, retryCount, doRefresh]);

  if (!pipelineRunning && !payload?.running && !streamError) {
    return null;
  }

  const p = payload;
  const showRunning = p?.running === true;
  const connecting = pipelineRunning && p == null && !streamError;

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
      {streamError && (
        <Typography variant="body2" color="error">
          {streamError}
        </Typography>
      )}
      {!streamError && connecting && (
        <Box sx={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 2 }}>
          <Typography variant="body2" color="text.secondary">
            Connecting to live progress…
          </Typography>
          <Button
            size="small"
            color="error"
            variant="outlined"
            disabled={stopLoading}
            onClick={handleStopPipeline}
          >
            {stopLoading ? 'Stopping…' : 'Stop pipeline'}
          </Button>
        </Box>
      )}
      {!streamError && !connecting && !showRunning && (
        <Typography variant="body2" color="text.secondary">
          No pipeline in progress.
        </Typography>
      )}
      {!streamError && showRunning && p && (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
          {reconnectingBuffered && (
            <Typography variant="caption" color="text.secondary" sx={{ fontStyle: 'italic' }}>
              Reconnecting… (showing last status)
            </Typography>
          )}
          {p.detail && (
            <Typography variant="body2" color="text.secondary">
              {p.detail}
            </Typography>
          )}
          {p.params && Object.keys(p.params).length > 0 && (
            <Typography variant="caption" color="text.secondary" component="div">
              {Object.entries(p.params)
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
          )}
          {p.step_id === 'auto_tune' && p.auto_tune && (
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.75, mt: 0.5 }}>
              <Typography variant="caption" fontWeight={600} color="primary.main">
                Phase: {p.auto_tune.phase === 'fine_tuning' ? 'Fine-tuning' : p.auto_tune.phase === 'loading' ? 'Loading' : p.auto_tune.phase === 'pycaret' ? 'PyCaret ranking' : p.auto_tune.phase === 'autogluon' ? 'AutoGluon' : 'Algorithm screening'}
                {p.auto_tune.activity && (
                  <> · Activity: {formatActivity(p.auto_tune.activity)}</>
                )}
              </Typography>
              {p.auto_tune.algorithms_requested && p.auto_tune.algorithms_requested.length > 0 && (
                <Typography variant="caption" color="text.secondary">
                  Selected: <strong>{p.auto_tune.algorithms_requested.join(', ')}</strong>
                </Typography>
              )}
              {p.auto_tune.algorithms_screened && p.auto_tune.algorithms_screened.length > 0 && (
                <Typography variant="caption" color="text.secondary">
                  Considering: {p.auto_tune.algorithms_screened.join(', ')}
                  {p.auto_tune.format_suffix && ` · Format: ${p.auto_tune.format_suffix}`}
                </Typography>
              )}
              {p.auto_tune.algorithm && !p.auto_tune.algorithms_screened?.length && (
                <Typography variant="caption" color="text.secondary">
                  Current algorithm: <strong>{p.auto_tune.algorithm}</strong>
                  {p.auto_tune.format_suffix && ` · Format: ${p.auto_tune.format_suffix}`}
                </Typography>
              )}
              {p.auto_tune.hyperparams && Object.keys(p.auto_tune.hyperparams).length > 0 && (
                <Typography variant="caption" color="text.secondary" component="div">
                  Hyperparams:{' '}
                  {Object.entries(p.auto_tune.hyperparams)
                    .map(([k, v]) => `${k}=${String(v)}`)
                    .join(', ')}
                </Typography>
              )}
              {(p.auto_tune.trial != null || p.auto_tune.trials_total != null) && (
                <Typography variant="caption" color="text.secondary">
                  Trial {p.auto_tune.trial ?? '?'} / {p.auto_tune.trials_total ?? '?'}
                </Typography>
              )}
              {p.auto_tune.best_score != null && (
                <Typography variant="caption" color="text.secondary">
                  Best score so far: {p.auto_tune.best_score.toFixed(4)}
                </Typography>
              )}
              {p.auto_tune.message && (
                <Typography variant="caption" color="text.secondary">
                  {p.auto_tune.message}
                </Typography>
              )}
            </Box>
          )}
          {p.step_id === 'auto_tune' && !p.auto_tune?.phase && (
            <Typography variant="caption" color="text.secondary" sx={{ fontStyle: 'italic' }}>
              Algorithms are screened first; best algorithm is then fine-tuned. Progress updates as tuning runs.
            </Typography>
          )}
          <Box sx={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 2 }}>
            <Typography variant="body2" fontWeight={600}>
              {p.step_label || p.step_id || 'Running'}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Elapsed: {formatElapsed(p.elapsed_sec ?? 0)}
            </Typography>
            {p.estimated_remaining_sec != null && p.estimated_remaining_sec > 0 && (
              <Typography variant="body2" color="text.secondary">
                Est. remaining: ~{formatElapsed(p.estimated_remaining_sec)}
              </Typography>
            )}
            <Button
              size="small"
              color="error"
              variant="outlined"
              disabled={stopLoading}
              onClick={handleStopPipeline}
            >
              {stopLoading ? 'Stopping…' : 'Stop pipeline'}
            </Button>
          </Box>
          {stopError && (
            <Typography variant="caption" color="error">
              {stopError}
            </Typography>
          )}
          {p.precompute && (
            <Box>
              <Typography variant="caption" color="text.secondary" display="block">
                Phase: {p.precompute.phase || '—'} · Current format:{' '}
                {p.precompute.current_format || '—'}
              </Typography>
              {p.precompute.formats_total != null && p.precompute.formats_total > 0 && (
                <Box sx={{ mt: 0.5 }}>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.25 }}>
                    <Typography variant="caption" color="text.secondary">
                      Formats: {p.precompute.formats?.join(', ') || '—'}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      {p.precompute.current_index != null && p.precompute.current_index >= 0
                        ? `${p.precompute.current_index + 1} / ${p.precompute.formats_total}`
                        : `0 / ${p.precompute.formats_total}`}
                    </Typography>
                  </Box>
                  <LinearProgress
                    variant="determinate"
                    value={
                      p.precompute.current_index != null && p.precompute.formats_total > 0
                        ? ((p.precompute.current_index + 1) / p.precompute.formats_total) * 100
                        : 0
                    }
                    sx={{ height: 6, borderRadius: 1 }}
                  />
                </Box>
              )}
            </Box>
          )}
        </Box>
      )}
    </Box>
  );
};

export default PipelineProgressPanel;
