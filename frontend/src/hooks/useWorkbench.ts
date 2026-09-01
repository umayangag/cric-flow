import { useCallback, useMemo, useState } from 'react';
import { api } from '../api';
import { useAsync } from './useAsync';
import type { ApiError } from '../lib/apiError';
import type { ModelMetadataApiResponse, ModelStatsResponse, WalkForwardRegistry } from '../types';

export interface UseWorkbenchReturn {
  registryFile: File | null;
  registryError: string | null;
  registry: WalkForwardRegistry | null;
  handleRegistryFile: (e: React.ChangeEvent<HTMLInputElement>) => void;
  /** Full model metadata from GET /api/ml/model-metadata; null until loaded or on error. No fallback. */
  modelMetadata: ModelMetadataApiResponse | null;
  modelMetadataLoading: boolean;
  modelMetadataError: ApiError | null;
  modelStats: ModelStatsResponse | null;
  modelStatsLoading: boolean;
  modelStatsError: ApiError | null;
}

/**
 * The Workbench's state: what each loaded model *is*.
 *
 * How well the models predict moved to the Evaluation report tab, which reads L4's own
 * measurements. The accuracy trend that used to live here re-scored one match at a time
 * against the batting, bowling and fielding models, and went with them in P-5.
 */
export function useWorkbench(): UseWorkbenchReturn {
  const metadata = useAsync(api.getModelMetadata, {
    runOnMount: [],
    errorMessage: 'Failed to load model metadata',
  });
  // Model stats carry each model's provenance and whether its dataset is still live
  // (ops plan P-2). Fetched here rather than in the section so the Workbench makes one
  // request whatever it chooses to render.
  const stats = useAsync(api.getModelStats, {
    runOnMount: [],
    errorMessage: 'Failed to load model stats',
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
      modelMetadata: metadata.data,
      modelMetadataLoading: metadata.loading,
      modelMetadataError: metadata.error,
      modelStats: stats.data,
      modelStatsLoading: stats.loading,
      modelStatsError: stats.error,
    }),
    [
      registryFile,
      registryError,
      registry,
      handleRegistryFile,
      metadata.data,
      metadata.loading,
      metadata.error,
      stats.data,
      stats.loading,
      stats.error,
    ],
  );
}
