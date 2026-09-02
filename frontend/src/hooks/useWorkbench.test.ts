import { describe, it, expect, beforeEach, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useWorkbench } from './useWorkbench';

vi.mock('../api', () => ({
  api: {
    getModelMetadata: vi.fn().mockResolvedValue({}),
    getModelStats: vi.fn().mockResolvedValue({ models: [] }),
  },
}));

/** A change event carrying one file, as the file input produces it. */
function fileEvent(contents: string) {
  const file = new File([contents], 'registry.json', { type: 'application/json' });
  return { target: { files: [file] } } as unknown as React.ChangeEvent<HTMLInputElement>;
}

describe('useWorkbench', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('accepts a walk-forward registry', async () => {
    const { result } = renderHook(() => useWorkbench());

    act(() => result.current.handleRegistryFile(fileEvent('{"windows": [{"format": "T20"}]}')));

    await waitFor(() => expect(result.current.registry).not.toBeNull());
    expect(result.current.registryError).toBeNull();
    expect(result.current.registryFile?.name).toBe('registry.json');
  });

  it('states the shape it expected when the file is the wrong one', async () => {
    const { result } = renderHook(() => useWorkbench());

    act(() => result.current.handleRegistryFile(fileEvent('{"folds": []}')));

    await waitFor(() => expect(result.current.registryError).not.toBeNull());
    expect(result.current.registryError).toMatch(/"windows" array/);
    expect(result.current.registry).toBeNull();
  });

  it('reports unparseable JSON rather than swallowing it', async () => {
    const { result } = renderHook(() => useWorkbench());

    act(() => result.current.handleRegistryFile(fileEvent('{not json')));

    await waitFor(() => expect(result.current.registryError).not.toBeNull());
    expect(result.current.registry).toBeNull();
  });

  it('clears the previous file when the picker is cancelled', async () => {
    const { result } = renderHook(() => useWorkbench());
    act(() => result.current.handleRegistryFile(fileEvent('{"windows": []}')));
    await waitFor(() => expect(result.current.registry).not.toBeNull());

    act(() =>
      result.current.handleRegistryFile({
        target: { files: [] },
      } as unknown as React.ChangeEvent<HTMLInputElement>),
    );

    expect(result.current.registryFile).toBeNull();
    expect(result.current.registry).toBeNull();
  });
});
