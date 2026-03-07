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
      expect(steps).toHaveLength(10);
      expect(steps.map((s) => s.id)).toContain('import');
      expect(steps.map((s) => s.id)).toContain('precompute');
      expect(steps.map((s) => s.id)).toContain('auto_tune');
      steps.forEach((s) => {
        expect(s.label).toBeTruthy();
        expect(s.command).toBeTruthy();
        expect(s.description).toBeTruthy();
      });
      const importStep = steps.find((s) => s.id === 'import');
      expect(importStep?.status).toBe('pending');
      expect(importStep?.runnable).toBe(true);
    });

    it('returns default steps when data is empty object', () => {
      const steps = derivePipelineSteps({ timestamp: '' });
      expect(steps).toHaveLength(10);
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

    it('sets precompute to success when at least one format has status ok', () => {
      const steps = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        precompute: { formats: { T20: { status: 'ok' } } },
      } as Parameters<typeof derivePipelineSteps>[0]);
      const precomputeStep = steps.find((s) => s.id === 'precompute');
      expect(precomputeStep?.status).toBe('success');
    });

    it('sets precompute to stale when format has status stale', () => {
      const steps = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        precompute: { formats: { T20: { status: 'stale' } } },
      } as Parameters<typeof derivePipelineSteps>[0]);
      const precomputeStep = steps.find((s) => s.id === 'precompute');
      expect(precomputeStep?.status).toBe('stale');
    });

    it('sets export to success when at least one format has file with exists true', () => {
      const steps = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        exports: { formats: { T20: { files: [{ exists: true }] } } },
      } as Parameters<typeof derivePipelineSteps>[0]);
      const exportStep = steps.find((s) => s.id === 'export');
      expect(exportStep?.status).toBe('success');
    });

    it('sets batting/bowling/fielding to success when artifact loaded or exists', () => {
      const steps = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        artifacts: {
          formats: {
            T20: {
              batting: { loaded: true },
              bowling: { exists: true },
              fielding: { loaded: true },
            },
          },
        },
      } as Parameters<typeof derivePipelineSteps>[0]);
      expect(steps.find((s) => s.id === 'train_batting')?.status).toBe('success');
      expect(steps.find((s) => s.id === 'train_bowling')?.status).toBe('success');
      expect(steps.find((s) => s.id === 'train_fielding')?.status).toBe('success');
    });

    it('overrides step status from pipeline.steps (running, completed, runnable)', () => {
      const steps = derivePipelineSteps({
        timestamp: '2026-01-01T00:00:00Z',
        services: { api_readiness: true },
        db: { counts: { matches: 1 } },
        pipeline: {
          steps: {
            import: { completed: true, runnable: true },
            precompute: { running: true, runnable: false },
            train_batting: { completed: true, runnable: false },
          },
        },
      } as Parameters<typeof derivePipelineSteps>[0]);
      const importStep = steps.find((s) => s.id === 'import');
      const precomputeStep = steps.find((s) => s.id === 'precompute');
      const battingStep = steps.find((s) => s.id === 'train_batting');
      expect(importStep?.status).toBe('success');
      expect(importStep?.runnable).toBe(true); // import always runnable
      expect(precomputeStep?.status).toBe('running');
      expect(precomputeStep?.runnable).toBe(false);
      expect(battingStep?.status).toBe('success');
      expect(battingStep?.runnable).toBe(false);
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
});
