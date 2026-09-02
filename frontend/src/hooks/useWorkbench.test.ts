import { describe, it, expect, beforeEach, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useWorkbench } from './useWorkbench';

const mockXiStatus = vi.fn();
vi.mock('../api', () => ({
  api: {
    xiStatus: (...args: unknown[]) => mockXiStatus(...args),
  },
}));

const LOADED = {
  loaded: true,
  formats: ['T20'],
  players: 13569,
  ratings_through: '2026-08-30',
  run_id: '20260902T101500Z-ab12cd34',
};

/**
 * The hook is one request wide since F-1 (D-8): the walk-forward registry upload it
 * also carried asked for a file no module has written since P-5, and walk-forward
 * numbers come from L4's report on the Evaluation tab.
 */
describe('useWorkbench', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('asks for the loaded run once and reports what it is', async () => {
    mockXiStatus.mockResolvedValue(LOADED);

    const { result } = renderHook(() => useWorkbench());

    await waitFor(() => expect(result.current.runStatus).not.toBeNull());
    expect(mockXiStatus).toHaveBeenCalledTimes(1);
    expect(result.current.runStatus?.run_id).toBe('20260902T101500Z-ab12cd34');
    expect(result.current.runStatusLoading).toBe(false);
    expect(result.current.runStatusError).toBeNull();
  });

  it('surfaces the failure rather than reporting no run', async () => {
    mockXiStatus.mockRejectedValue(new Error('Connection refused'));

    const { result } = renderHook(() => useWorkbench());

    await waitFor(() => expect(result.current.runStatusError).not.toBeNull());
    expect(result.current.runStatus).toBeNull();
  });
});
