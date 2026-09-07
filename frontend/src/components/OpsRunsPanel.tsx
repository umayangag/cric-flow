import React from 'react';
import { Chip, Stack, Typography } from '@mui/material';
import { RATINGS_STALE_CODE } from '../types';
import SectionCard from './common/SectionCard';
import StatusPill from './common/StatusPill';
import {
  asObj,
  freshnessPillState,
  readFreshness,
  servedFreshnessLabel,
} from '../utils/opsStatusHelpers';
import type { OpsStatus } from '../utils/opsStatusHelpers';

/** One run directory, as go-app /ops/status copies it through from ml-service. */
type RunRow = {
  run_id?: string;
  created_at?: string;
  cutoff?: string;
  git_sha?: string;
  dataset_sha?: string;
  formats?: string[];
  has_manifest?: boolean;
  current?: boolean;
  loaded?: boolean;
};

/**
 * Which run is serving, and which runs exist (H-16).
 *
 * It replaced a formats-by-model-kind matrix, because a run is what an artifact belongs
 * to now: "is the model current?" is answered by which run `current` points at and
 * whether that is the run the process loaded, not by six per-format files that could
 * each have come from a different training session.
 *
 * Two states have to be visible and were not before. A run the loader *refused* — no
 * manifest, or arrays this code cannot serve (D-6) — appears with the reason instead of
 * looking like a box that has never trained. And ratings too old to answer a live
 * request with (H-11) are shown as the verdict, not as a date the reader has to judge.
 */
const OpsRunsPanel: React.FC<{ data: OpsStatus }> = ({ data }) => {
  const artifacts = asObj(data.artifacts);
  const runs = (Array.isArray(artifacts.runs) ? artifacts.runs : []) as RunRow[];
  const loadedRun = typeof artifacts.loaded_run === 'string' ? artifacts.loaded_run : null;
  const currentRun = typeof artifacts.current_run === 'string' ? artifacts.current_run : null;
  const error = typeof artifacts.error === 'string' ? artifacts.error : '';
  // The verdict comes off the one freshness object, never off this section's own copy of
  // ml-service's answer: one badge, one rule, one date (P2-1).
  const served = readFreshness(data).served;

  return (
    <SectionCard
      title="Runs"
      subtitle="Every training run on disk. `current` is a pointer to one of them; Reload moves it."
    >
      <Stack spacing={1.5}>
        <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
          <Typography variant="body2">Serving:</Typography>
          <StatusPill state={loadedRun ? 'ok' : 'missing'} label={loadedRun ?? 'nothing loaded'} />
          <StatusPill
            state={freshnessPillState(served.status)}
            label={servedFreshnessLabel(served)}
          />
        </Stack>
        {served.status === 'stale' && (
          <Typography variant="body2" color="warning.dark">
            Live predictions are refused with <strong>{served.code ?? RATINGS_STALE_CODE}</strong>{' '}
            until a retrain and a reload (H-11).
          </Typography>
        )}
        {error && (
          <Typography variant="body2" color="error.dark">
            {error}
          </Typography>
        )}
        {runs.length === 0 ? (
          <Typography variant="body2" color="text.secondary">
            No runs yet. Run <strong>Retrain</strong> from the pipeline above.
          </Typography>
        ) : (
          <Stack spacing={0.75}>
            {runs.map((run) => (
              <Stack
                key={run.run_id}
                direction="row"
                alignItems="center"
                justifyContent="space-between"
                spacing={1}
              >
                <Typography variant="body2" sx={{ fontFamily: 'ui-monospace, Menlo, monospace' }}>
                  {run.run_id}
                </Typography>
                <Stack direction="row" spacing={0.5} alignItems="center">
                  {run.cutoff && <Chip size="small" label={`cutoff ${run.cutoff}`} />}
                  {run.git_sha && <Chip size="small" label={run.git_sha.slice(0, 7)} />}
                  {run.has_manifest === false && <StatusPill state="error" label="no manifest" />}
                  {(run.run_id === currentRun || run.current) && (
                    <StatusPill state="ok" label="current" />
                  )}
                  {(run.run_id === loadedRun || run.loaded) && (
                    <StatusPill state="ok" label="loaded" />
                  )}
                </Stack>
              </Stack>
            ))}
          </Stack>
        )}
      </Stack>
    </SectionCard>
  );
};

export default OpsRunsPanel;
