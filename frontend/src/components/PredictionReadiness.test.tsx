import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import PredictionReadiness from './PredictionReadiness';
import type { OpsStatus } from '../utils/opsStatusHelpers';

/** An /ops/status payload where the named steps have completed successfully. */
function statusWith(completed: string[]): OpsStatus {
  const steps: Record<string, { completed: boolean }> = {};
  for (const id of ['import', 'precompute', 'export']) {
    steps[id] = { completed: completed.includes(id) };
  }
  return { timestamp: '2026-08-28T00:00:00Z', pipeline: { steps } } as unknown as OpsStatus;
}

describe('PredictionReadiness', () => {
  it('says nothing when the pipeline is ready', () => {
    const { container } = render(
      <PredictionReadiness status={statusWith(['import', 'precompute', 'export'])} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it('says nothing before the status is known, rather than guessing', () => {
    const { container } = render(<PredictionReadiness status={null} />);
    expect(container).toBeEmptyDOMElement();
  });

  /**
   * The failure W4-2 is about: without precompute there are no feature snapshots, the
   * vector is zero-filled, and the models answer anyway. Nothing errors.
   */
  it('warns that features will be zero-filled when precompute has not run', () => {
    render(<PredictionReadiness status={statusWith(['import'])} />);
    expect(screen.getByText(/filled with zeros/)).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('precompute');
  });

  it('does not mention zero-filling when only export is outstanding', () => {
    render(<PredictionReadiness status={statusWith(['import', 'precompute'])} />);
    expect(screen.queryByText(/filled with zeros/)).not.toBeInTheDocument();
    expect(screen.getByText(/export/)).toBeInTheDocument();
  });
});
