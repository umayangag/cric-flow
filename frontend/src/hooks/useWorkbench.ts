import { useCallback, useMemo, useState } from 'react';
import { api } from '../api';
import { useAsync } from './useAsync';
import type { ApiError } from '../lib/apiError';
import type {
  AccuracyTrendResponse,
  AccuracyTrendFilters,
  ModelMetadataApiResponse,
  ModelStatsResponse,
  WalkForwardRegistry,
} from '../types';

const DEFAULT_LIMIT = 100;
const MAX_LIMIT = 500;

export interface UseWorkbenchReturn {
  format: string;
  setFormat: (v: string) => void;
  startDate: string;
  setStartDate: (v: string) => void;
  endDate: string;
  setEndDate: (v: string) => void;
  limit: number;
  setLimit: (v: number) => void;
  maxLimit: number;
  availableFormats: string[];
  trendLoading: boolean;
  trendError: ApiError | null;
  trendData: AccuracyTrendResponse | null;
  loadAccuracyTrend: () => Promise<void>;
  registryFile: File | null;
  registryError: string | null;
  registry: WalkForwardRegistry | null;
  handleRegistryFile: (e: React.ChangeEvent<HTMLInputElement>) => void;
  /** Full model metadata from GET /api/ml/model-metadata (model_modes + entries); null until loaded or on error. No fallback. */
  modelMetadata: ModelMetadataApiResponse | null;
  modelMetadataLoading: boolean;
  modelMetadataError: ApiError | null;
  modelStats: ModelStatsResponse | null;
  modelStatsLoading: boolean;
  modelStatsError: ApiError | null;
}

/**
 * The Workbench's state, and the first surface migrated to {@link useAsync} (W1-1).
 *
 * It was chosen to prove the shape because it was the clearest case of the problem:
 * four independent fetches, each with its own `data`/`loading`/`error` triple written
 * out by hand, three of them with a near-identical mount effect and a
 * cancelled/active flag spelled differently every time. Twelve `useState` calls became
 * four `useAsync` calls, and the staleness guard the hand-written versions all lacked
 * came with them.
 *
 * What did *not* move: the four form fields, which are genuinely component state, and
 * the registry file, which is parsed locally rather than fetched.
 */
export function useWorkbench(): UseWorkbenchReturn {
  const [format, setFormat] = useState<string>('');
  const [startDate, setStartDate] = useState<string>('');
  const [endDate, setEndDate] = useState<string>('');
  const [limit, setLimit] = useState<number>(DEFAULT_LIMIT);

  const formats = useAsync(api.getFormats, { runOnMount: [] });
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
  const trend = useAsync((filters: AccuracyTrendFilters) => api.accuracyTrend(filters));

  const [registryFile, setRegistryFile] = useState<File | null>(null);
  const [registryError, setRegistryError] = useState<string | null>(null);
  const [registry, setRegistry] = useState<WalkForwardRegistry | null>(null);

  const loadAccuracyTrend = useCallback(async () => {
    await trend.run({
      format: format || undefined,
      start_date: startDate || undefined,
      end_date: endDate || undefined,
      order: 'asc',
      limit: Math.min(Math.max(1, limit), MAX_LIMIT),
      cache: 'read',
    });
  }, [trend, format, startDate, endDate, limit]);

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
          setRegistryError('Invalid registry: missing "windows" array');
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
      format,
      setFormat,
      startDate,
      setStartDate,
      endDate,
      setEndDate,
      limit,
      setLimit,
      maxLimit: MAX_LIMIT,
      // A failed format list is not worth a message here: the picker simply offers
      // nothing, and the panels below say why they are empty.
      availableFormats: formats.data ?? [],
      trendLoading: trend.loading,
      trendError: trend.error,
      trendData: trend.data,
      loadAccuracyTrend,
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
      format,
      startDate,
      endDate,
      limit,
      formats.data,
      trend.loading,
      trend.error,
      trend.data,
      loadAccuracyTrend,
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
