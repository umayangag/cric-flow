import { describe, it, expect } from 'vitest';
import { derivePipelineSteps, getStatusColor, statusIcon, type StepStatus } from './pipelineSteps';

describe('pipelineSteps', () => {
  describe('getStatusColor', () => {
    const cases: { status: StepStatus; want: string }[] = [
      { status: 'success', want: 'success.main' },
      { status: 'stale', want: 'warning.main' },
      { status: 'pending', want: 'text.secondary' },
      { status: 'error', want: 'error.main' },
      { status: 'optional', want: 'info.main' },
      { status: 'running', want: 'primary.main' },
    ];
    it.each(cases)('returns correct color for $status', ({ status, want }) => {
      expect(getStatusColor(status)).toBe(want);
    });

    it('returns text.secondary for unknown status (default branch)', () => {
      expect(getStatusColor('unknown' as StepStatus)).toBe('text.secondary');
    });
  });

  describe('statusIcon', () => {
    it('has entry for every non-running step status', () => {
      expect(statusIcon.success).toBe('✓');
      expect(statusIcon.stale).toBe('◐');
      expect(statusIcon.pending).toBe('○');
      expect(statusIcon.error).toBe('✗');
      expect(statusIcon.optional).toBe('◇');
    });
  });

  describe('derivePipelineSteps', () => {
    it('returns default steps when data is null', () => {
      const steps = derivePipelineSteps(null);
      expect(steps.map((s) => s.id)).toEqual(['import', 'retrain', 'evaluate', 'reload']);
      steps.forEach((s) => {
        expect(s.label).toBeTruthy();
        expect(s.command).toBeTruthy();
        expect(s.description).toBeTruthy();
      });
      const importStep = steps.find((s) => s.id === 'import');
      expect(importStep?.status).toBe('pending');
      expect(importStep?.runnable).toBe(true);
    });

    it('offers evaluate but never implies it', () => {
      const evaluate = derivePipelineSteps(null).find((s) => s.id === 'evaluate');
      expect(evaluate?.status).toBe('optional');
    });

    it('returns default steps when data is empty object', () => {
      const steps = derivePipelineSteps({ timestamp: '' });
      expect(steps).toHaveLength(4);
      expect(steps[0].status).toBe('pending');
    });

    it('sets import to success when api_readiness and matches > 0', () => {
      const steps = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        services: { api_readiness: true },
        db: { counts: { matches: 1 } },
      } as Parameters<typeof derivePipelineSteps>[0]);
      const importStep = steps.find((s) => s.id === 'import');
      expect(importStep?.status).toBe('success');
    });

    it('keeps import pending when matches is 0', () => {
      const steps = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        services: { api_readiness: true },
        db: { counts: { matches: 0 } },
      } as Parameters<typeof derivePipelineSteps>[0]);
      const importStep = steps.find((s) => s.id === 'import');
      expect(importStep?.status).toBe('pending');
    });

    it('sets retrain to success when a run with a manifest is on disk', () => {
      const steps = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        artifacts: { runs: [{ run_id: 'r1', has_manifest: true }] },
      } as Parameters<typeof derivePipelineSteps>[0]);
      expect(steps.find((s) => s.id === 'retrain')?.status).toBe('success');
    });

    it('leaves retrain pending when the only run directory is not a run', () => {
      const steps = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        artifacts: { runs: [{ run_id: 'r1', has_manifest: false }] },
      } as Parameters<typeof derivePipelineSteps>[0]);
      expect(steps.find((s) => s.id === 'retrain')?.status).toBe('pending');
    });

    it('sets reload to success only when a run is actually loaded', () => {
      const loaded = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        artifacts: { runs: [{ run_id: 'r1' }], loaded_run: 'r1' },
      } as Parameters<typeof derivePipelineSteps>[0]);
      const notLoaded = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        artifacts: { runs: [{ run_id: 'r1' }], loaded_run: null },
      } as Parameters<typeof derivePipelineSteps>[0]);
      expect(loaded.find((s) => s.id === 'reload')?.status).toBe('success');
      expect(notLoaded.find((s) => s.id === 'reload')?.status).toBe('pending');
    });

    it('overrides step status from pipeline.steps (running, completed, runnable)', () => {
      const steps = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        services: { api_readiness: true },
        db: { counts: { matches: 1 } },
        pipeline: {
          steps: {
            import: { completed: true, runnable: true },
            retrain: { running: true, runnable: false },
            reload: { completed: true, runnable: false },
          },
        },
      } as Parameters<typeof derivePipelineSteps>[0]);
      const importStep = steps.find((s) => s.id === 'import');
      const retrainStep = steps.find((s) => s.id === 'retrain');
      const reloadStep = steps.find((s) => s.id === 'reload');
      expect(importStep?.status).toBe('success');
      expect(importStep?.runnable).toBe(true); // import always runnable
      expect(retrainStep?.status).toBe('running');
      expect(retrainStep?.runnable).toBe(false);
      expect(reloadStep?.status).toBe('success');
      expect(reloadStep?.runnable).toBe(false);
    });

    /**
     * The regression test for a graph that stayed green after a cancelled run: the
     * heuristics above read files that outlive the run which wrote them, so where run
     * history says a step has not completed, the graph must say so too rather than
     * trusting the leftovers.
     */
    it('downgrades a step to stale when the backend says it has not completed', () => {
      const steps = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        services: { api_readiness: true },
        db: { counts: { matches: 1 } },
        artifacts: { runs: [{ run_id: 'r1', has_manifest: true }], loaded_run: 'r1' },
        pipeline: {
          steps: {
            retrain: { completed: false, runnable: true },
            reload: { completed: false, runnable: false },
          },
        },
      } as Parameters<typeof derivePipelineSteps>[0]);
      expect(steps.find((s) => s.id === 'retrain')?.status).toBe('stale');
      expect(steps.find((s) => s.id === 'reload')?.status).toBe('stale');
    });

    it('leaves a step pending when the backend says not completed and nothing is on disk', () => {
      const steps = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        pipeline: { steps: { reload: { completed: false, runnable: false } } },
      } as Parameters<typeof derivePipelineSteps>[0]);
      expect(steps.find((s) => s.id === 'reload')?.status).toBe('pending');
    });

    it('keeps running ahead of the downgrade for a step being re-run', () => {
      const steps = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        artifacts: { runs: [{ run_id: 'r1', has_manifest: true }] },
        pipeline: { steps: { retrain: { running: true, completed: false } } },
      } as Parameters<typeof derivePipelineSteps>[0]);
      expect(steps.find((s) => s.id === 'retrain')?.status).toBe('running');
    });

    it('leaves steps the backend does not report alone', () => {
      const steps = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        artifacts: { runs: [{ run_id: 'r1', has_manifest: true }] },
        pipeline: { steps: {} },
      } as Parameters<typeof derivePipelineSteps>[0]);
      // An older API that sends no per-step entry must not turn the whole graph amber.
      expect(steps.find((s) => s.id === 'retrain')?.status).toBe('success');
    });

    it('always keeps import step runnable', () => {
      const steps = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        pipeline: { steps: { import: { runnable: false } } },
      } as Parameters<typeof derivePipelineSteps>[0]);
      const importStep = steps.find((s) => s.id === 'import');
      expect(importStep?.runnable).toBe(true);
    });
  });

  /**
   * These descriptions are the last hand-written prose about the pipeline that still
   * ships inside the app (W2-1 deleted the other 214 lines). Prose drifts silently,
   * and this one already had: it still described "unified (legacy)" models after the
   * pooled tier was removed, and "form, consistency" features after the v3 contract
   * replaced them with raw windowed stats.
   *
   * A test cannot check that a description is *accurate*. It can check that it does
   * not name a concept the repo has removed, which is how every drift here has looked.
   * Adding a term to this list is the last step of removing a concept.
   */
  describe('descriptions do not describe a system that no longer exists', () => {
    const RETIRED = [
      // The pooled cross-format serving tier. Removed in C3-2 / consumer W0-1.
      { term: 'unified model', why: 'models are per-format; there is no pooled tier' },
      { term: 'legacy', why: 'the legacy artifact tier was removed with the pooled model' },
      // Formula features replaced by raw windowed stats in the v3 feature contract.
      { term: 'consistency', why: 'the v3 feature contract uses raw windowed stats' },
      // Planned, not implemented: docs/weather-not-implemented.md, consumer W0-3.
      { term: 'weather', why: 'nothing populates weather_data and no model reads it' },
      // The producers P-6 deleted, and the search it replaced with a fixed grid.
      {
        term: 'precompute',
        why: 'the rating pass reads ball_event directly; there is no precompute',
      },
      {
        term: 'auto-tune',
        why: 'the grid runs inside retrain and records its choice in the manifest',
      },
      { term: 'csv', why: 'nothing exports CSVs; the training frames live in the run directory' },
    ];

    it.each(RETIRED)('never says "$term" — $why', ({ term }) => {
      const offenders = derivePipelineSteps(null)
        .filter((step) => step.description.toLowerCase().includes(term))
        .map((step) => step.id);
      expect(offenders).toEqual([]);
    });
  });
});
