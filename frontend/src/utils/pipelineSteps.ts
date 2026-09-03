import type { OpsStatus } from '../components/OpsStatusTab';

/**
 * Every step the backend accepts at POST /ops/pipeline/run/{step}.
 *
 * This list is asserted against contracts/ops-console.contract.json, which is
 * generated from the go-app step registry. Adding a step on one side without the
 * other fails a test rather than shipping a UI that offers steps the backend
 * refuses — or hides steps it accepts.
 */
export type PipelineStepId = 'import' | 'retrain' | 'evaluate' | 'reload';

export type StepStatus = 'success' | 'stale' | 'pending' | 'error' | 'optional' | 'running';

export type PipelineStep = {
  id: PipelineStepId;
  label: string;
  status: StepStatus;
  /** The shell command that runs this step outside the console. */
  command: string;
  /**
   * What a run of this step is recorded as in `data_migrations`.
   *
   * Distinct from `command`, which is the make target a human would type. This is the
   * value run history carries, and it is what lets a surface recognise its own runs —
   * an accuracy trend that cannot tell a retrain from a precompute cannot say what
   * changed. Asserted against the generated contract, so it cannot drift from Go.
   */
  migrationCommand: string;
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

export function derivePipelineSteps(data: OpsStatus | null): PipelineStep[] {
  const steps: PipelineStep[] = [
    {
      id: 'import',
      label: 'Import',
      status: 'pending',
      command: 'make migrate && make cricsheet-import',
      migrationCommand: 'cricsheet-import',
      description:
        'Download the configured Cricsheet archive, extract it and load the matches into the ' +
        'database — fetch, extract and import as one run. Fetch and extract are skipped, and say ' +
        'so, when the dataset directory already holds that archive. Run from project root.',
      runnable: true,
    },
    {
      id: 'retrain',
      label: 'Retrain',
      status: 'pending',
      command: 'make retrain CUTOFF=2025-09-01',
      migrationCommand: 'xi-retrain',
      description:
        'The whole model build in one step: the rating pass over ball_event, the XI win models ' +
        '(with the small hyperparameter grid), the performance models, the run report and the run ' +
        'manifest. Everything lands in runs/<run_id>/ and nothing is published — Reload decides ' +
        'which run serves. Run from project root.',
      runnable: true,
    },
    {
      id: 'evaluate',
      label: 'Evaluate',
      status: 'optional',
      command: 'make evaluate',
      migrationCommand: 'xi-evaluate',
      description:
        "L4's evaluation harness: rolling-origin walk-forward plus the locked window, the selection " +
        'and performance metrics, the leak canary and the train/serve parity check. It writes a ' +
        'report and touches no artifact `current` points at, which is why it sits beside the ' +
        'pipeline rather than in it. Optional, and slow: it refits every model per fold per format.',
      runnable: true,
    },
    {
      id: 'reload',
      label: 'Reload',
      status: 'pending',
      command: 'make reload',
      migrationCommand: 'xi-reload',
      description:
        'Point `current` at a run and load it into the running ML service. With no run id it loads ' +
        'the newest run on disk — so Retrain followed by Reload serves the run just built. ' +
        'Naming a run is how you swap back to an earlier one.',
      runnable: true,
    },
  ];

  if (!data) return steps;

  const db = asObj(data.db);
  const counts = asObj(db.counts);
  const matchesCount = typeof counts.matches === 'number' ? counts.matches : 0;
  const importDone = data.services?.api_readiness === true && matchesCount > 0;

  // A run on disk means a retrain produced one; a *loaded* run means a reload served it.
  // Both are evidence, and neither is a substitute for run history saying the step
  // completed -- see the loop below.
  const artifacts = asObj(data.artifacts);
  const runs = Array.isArray(artifacts.runs) ? artifacts.runs : [];
  const retrainDone = runs.some((r: unknown) => asObj(r).has_manifest !== false);
  const reloadDone = typeof artifacts.loaded_run === 'string' && artifacts.loaded_run.length > 0;

  steps[0].status = importDone ? 'success' : 'pending';
  steps[1].status = retrainDone ? 'success' : 'pending';
  steps[3].status = reloadDone ? 'success' : 'pending';
  //
  // The backend decides completion; the checks above only decide what to show for a
  // step it has said nothing about. They read artifacts on disk, which outlive the run
  // that produced them: a cancelled precompute left snapshots behind and the graph went
  // on showing a green tick, because this loop could raise a step to success but never
  // lower one. Where run history says a step has not completed, evidence on disk makes
  // it stale -- there is something there, just not from a run that finished -- and no
  // evidence makes it pending.
  const pipelineSteps = asObj(asObj(data.pipeline).steps);
  for (let i = 0; i < steps.length; i++) {
    const step = steps[i];
    const reported = Object.prototype.hasOwnProperty.call(pipelineSteps, step.id);
    const stepData = asObj(pipelineSteps[step.id]);
    const running = stepData.running === true;
    const completed = stepData.completed === true;
    if (running) steps[i].status = 'running';
    else if (completed) steps[i].status = 'success';
    else if (reported && steps[i].status === 'success') steps[i].status = 'stale';
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
