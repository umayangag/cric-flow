import { describe, it, expect, beforeEach, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useEvaluationReport } from './useEvaluationReport';
import type { EvaluationReport } from '../types';

const mockEvaluationReport = vi.fn();
vi.mock('../api', () => ({
  api: {
    evaluationReport: (...args: unknown[]) => mockEvaluationReport(...args),
  },
}));

const report = {
  generated_at: '2026-08-30T12:00:00+00:00',
  source: 'PostgresSource',
  cutoffs: [],
  locked_start: '2025-09-01',
  seeds: [0],
  n_rows: 1,
  n_player_rows: 1,
  formats: {
    T20: { n_matches: 10 },
    ODI: { n_matches: 5 },
  },
  serving_parity: { passed: true },
} as unknown as EvaluationReport;

describe('useEvaluationReport', () => {
  beforeEach(() => {
    mockEvaluationReport.mockReset();
    mockEvaluationReport.mockResolvedValue(report);
  });

  it('loads the report on mount and selects the first format', async () => {
    const { result } = renderHook(() => useEvaluationReport());

    await waitFor(() => expect(result.current.report).not.toBeNull());
    expect(result.current.formats).toEqual(['T20', 'ODI']);
    expect(result.current.format).toBe('T20');
    expect(result.current.formatReport?.n_matches).toBe(10);
  });

  it('follows a chosen format', async () => {
    const { result } = renderHook(() => useEvaluationReport());
    await waitFor(() => expect(result.current.report).not.toBeNull());

    act(() => result.current.setFormat('ODI'));

    expect(result.current.format).toBe('ODI');
    expect(result.current.formatReport?.n_matches).toBe(5);
  });

  it('falls back to the first format when the chosen one is not in the report', async () => {
    const { result } = renderHook(() => useEvaluationReport());
    await waitFor(() => expect(result.current.report).not.toBeNull());

    act(() => result.current.setFormat('T20I'));

    expect(result.current.format).toBe('T20');
  });

  it('reports a missing report rather than rendering an empty one', async () => {
    mockEvaluationReport.mockRejectedValue(new Error('no evaluation report'));
    const { result } = renderHook(() => useEvaluationReport());

    await waitFor(() => expect(result.current.error).not.toBeNull());
    expect(result.current.report).toBeNull();
    expect(result.current.formats).toEqual([]);
    expect(result.current.formatReport).toBeNull();
  });
});
