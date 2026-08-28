import type { OpsStatus } from '../components/OpsStatusTab';

/**
 * Every step the backend accepts at POST /ops/pipeline/run/{step}.
 *
 * This list is asserted against contracts/ops-console.contract.json, which is
 * generated from the go-app step registry. Adding a step on one side without the
 * other fails a test rather than shipping a UI that offers steps the backend
 * refuses — or hides steps it accepts.
 */
export type PipelineStepId =
  | 'import'
  | 'precompute'
  | 'export'
  | 'train_batting'
  | 'train_bowling'
  | 'train_fielding'
  | 'train_extras'
  | 'train_win'
  | 'train_innings'
  | 'train_combination_meta'
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
  /**
   * A precondition step ordering cannot express, phrased as something to do.
   * Shown in the step dialog so the operator reads it before the run fails.
   */
  prerequisite?: string;
};

function asObj(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' ? (v as Record<string, unknown>) : {};
}

function getFormats(section: unknown): Record<string, unknown> {
  const obj = asObj(section);
  return asObj(obj.formats);
}

export function derivePipelineSteps(data: OpsStatus | null): PipelineStep[] {
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
        'Compute raw windowed stat snapshots (and sequence features when enabled) for all formats. Run from project root.',
      runnable: true,
    },
    {
      id: 'export',
      label: 'Export',
      status: 'pending',
      command: 'make export-dataset',
      description:
        'Export all-format and per-format batting/bowling CSVs to output/go-app. Run from project root.',
      runnable: true,
    },
    {
      id: 'train_batting',
      label: 'Train Batting',
      status: 'pending',
      command: 'make train-batting',
      description:
        'Train per-format batting models from exported CSVs. Uses params from config and DB (from previous auto-tune when GO_APP_URL is set). Run from project root.',
      runnable: true,
    },
    {
      id: 'train_bowling',
      label: 'Train Bowling',
      status: 'pending',
      command: 'make train-bowling',
      description:
        'Train per-format bowling models from exported CSVs. Uses params from config and DB (from previous auto-tune when GO_APP_URL is set). Run from project root.',
      runnable: true,
    },
    {
      id: 'train_fielding',
      label: 'Train Fielding',
      status: 'pending',
      command: 'make train-fielding CUTOFF=2025-01-01T00:00:00Z',
      description:
        'Train per-format fielding models. Uses params from config and DB. Set CUTOFF (RFC3339) and GO_APP_URL; or use FIELDING_CSV=<path>. Run from project root.',
      runnable: true,
    },
    {
      id: 'train_extras',
      label: 'Train Extras',
      status: 'pending',
      command: 'make train-extras CUTOFF=2025-01-01T00:00:00Z',
      description:
        'Train per-format extras models. Uses params from config and DB. Set CUTOFF and GO_APP_URL; or EXTRAS_CSV=<path>. Run from project root.',
      runnable: true,
    },
    {
      id: 'train_win',
      label: 'Train Win',
      status: 'pending',
      command: 'make train-win CUTOFF=2025-01-01T00:00:00Z',
      description:
        'Train per-format win models. Uses params from config and DB. Set CUTOFF and GO_APP_URL; or WIN_CSV=<path>. Run from project root.',
      runnable: true,
    },
    {
      id: 'train_innings',
      label: 'Train Innings',
      status: 'pending',
      command: 'make train-innings CUTOFF=2025-01-01T00:00:00Z',
      description:
        'Train per-format innings models (innings_runs, innings_wickets) for reconciliation. Uses params from config and DB. Set CUTOFF and GO_APP_URL. Run from project root.',
      runnable: true,
    },
    {
      id: 'train_combination_meta',
      label: 'Train Combination Meta',
      status: 'optional',
      command: 'make train-combination-meta',
      description:
        'Train the score-combination meta-model that blends per-model contributions into a final score. Optional; run it after Train Win when you want blended scores rather than the default weights.',
      runnable: true,
      prerequisite:
        'Needs backtest_contributions.csv. Produce it with "Export contributions" on the Evaluate tab first — without it this step fails with CONTRIBUTIONS_CSV_MISSING.',
    },
    {
      id: 'auto_tune',
      label: 'Auto-tune',
      status: 'optional',
      command: 'make ml-auto-tune MODEL=all ALL_FORMATS=1',
      description:
        'Discover best algorithm and hyperparameters (saves to DB when GO_APP_URL is set). Run when params are unknown or you want to re-optimize. After auto-tune, you can run Train steps to refresh all artifacts from the new DB params. Optional; use Train steps only when params are already known.',
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

export const statusIcon: Record<Exclude<StepStatus, 'running'>, string> = {
  success: '✓',
  stale: '◐',
  pending: '○',
  error: '✗',
  optional: '◇',
};

export function getStatusColor(status: StepStatus): string {
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
}
