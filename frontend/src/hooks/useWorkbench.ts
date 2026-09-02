import { useCallback, useMemo, useState } from 'react';
import { api } from '../api';
import { useAsync } from './useAsync';
import type { ApiError } from '../lib/apiError';
import type { WalkForwardRegistry, XiStatusResponse } from '../types';

export interface UseWorkbenchReturn {
  registryFile: File | null;
  registryError: string | null;
  registry: WalkForwardRegistry | null;
  handleRegistryFile: (e: React.ChangeEvent<HTMLInputElement>) => void;
  /** The loaded run and its manifest, from GET /api/ml/xi-status. Null until loaded or on error. */
  runStatus: XiStatusResponse | null;
  runStatusLoading: boolean;
  runStatusError: ApiError | null;
}

/**
 * The Workbench's state: what each loaded model *is*.
 *
 * How well the models predict moved to the Evaluation report tab, which reads L4's own
 * measurements. The accuracy trend that used to live here re-scored one match at a time
 * against the batting, bowling and fielding models, and went with them in P-5.
 */
export function useWorkbench(): UseWorkbenchReturn {
  // One request: which run is loaded, and what its manifest says. Provenance used to be
  // read from a per-artifact sidecar and the feature list from the win model's metadata;
  // both were inferences about an artifact, and the manifest is the record (H-16).
  const runStatus = useAsync(api.xiStatus, {
    runOnMount: [],
    errorMessage: 'Failed to load the run status',
  });

  const [registryFile, setRegistryFile] = useState<File | null>(null);
  const [registryError, setRegistryError] = useState<string | null>(null);
  const [registry, setRegistry] = useState<WalkForwardRegistry | null>(null);

  const handleRegistryFile = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    setRegistryError(null);
    setRegistry(null);
    setRegistryFile(file || null);
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      try {
        const parsed = JSON.parse(reader.result as string) as WalkForwardRegistry;
        if (!parsed.windows || !Array.isArray(parsed.windows)) {
          // The shape is stated here rather than in prose above the upload button
          // (W2-2): it is only needed by someone whose file is wrong, and a rule
          // enforced next to the check cannot drift away from it.
          setRegistryError(
            'Not a walk-forward registry: expected a JSON object with a "windows" array, ' +
              'each entry carrying model_type, format, cutoff_trained_before, window_x and metrics.',
          );
          return;
        }
        setRegistry(parsed);
      } catch (err) {
        setRegistryError(err instanceof Error ? err.message : 'Invalid JSON');
      }
    };
    reader.readAsText(file);
  }, []);

  return useMemo(
    () => ({
      registryFile,
      registryError,
      registry,
      handleRegistryFile,
      runStatus: runStatus.data,
      runStatusLoading: runStatus.loading,
      runStatusError: runStatus.error,
    }),
    [
      registryFile,
      registryError,
      registry,
      handleRegistryFile,
      runStatus.data,
      runStatus.loading,
      runStatus.error,
    ],
  );
}
