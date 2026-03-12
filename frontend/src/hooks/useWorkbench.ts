import { useState, useEffect, useCallback } from 'react';
import { api } from '../api';
import type {
  AccuracyTrendResponse,
  AccuracyTrendFilters,
  ModelMetadataApiResponse,
  WalkForwardRegistry,
} from '../types';

const DEFAULT_LIMIT = 100;
const MAX_LIMIT = 500;

export interface UseWorkbenchReturn {
  format: string;
  setFormat: (v: string) => void;
  predictionModel: 'format' | 'unified';
  setPredictionModel: (v: 'format' | 'unified') => void;
  startDate: string;
  setStartDate: (v: string) => void;
  endDate: string;
  setEndDate: (v: string) => void;
  limit: number;
  setLimit: (v: number) => void;
  maxLimit: number;
  availableFormats: string[];
  trendLoading: boolean;
  trendError: string | null;
  trendData: AccuracyTrendResponse | null;
  loadAccuracyTrend: () => Promise<void>;
  registryFile: File | null;
  registryError: string | null;
  registry: WalkForwardRegistry | null;
  handleRegistryFile: (e: React.ChangeEvent<HTMLInputElement>) => void;
  /** Full model metadata from GET /api/ml/model-metadata (model_modes + entries); null until loaded or on error. No fallback. */
  modelMetadata: ModelMetadataApiResponse | null;
  modelMetadataLoading: boolean;
  modelMetadataError: string | null;
}

export function useWorkbench(): UseWorkbenchReturn {
  const [format, setFormat] = useState<string>('');
  const [predictionModel, setPredictionModel] = useState<'format' | 'unified'>('format');
  const [startDate, setStartDate] = useState<string>('');
  const [endDate, setEndDate] = useState<string>('');
  const [limit, setLimit] = useState<number>(DEFAULT_LIMIT);
  const [availableFormats, setAvailableFormats] = useState<string[]>([]);
  const [trendLoading, setTrendLoading] = useState(false);
  const [trendError, setTrendError] = useState<string | null>(null);
  const [trendData, setTrendData] = useState<AccuracyTrendResponse | null>(null);

  const [registryFile, setRegistryFile] = useState<File | null>(null);
  const [registryError, setRegistryError] = useState<string | null>(null);
  const [registry, setRegistry] = useState<WalkForwardRegistry | null>(null);

  const [modelMetadata, setModelMetadata] = useState<ModelMetadataApiResponse | null>(null);
  const [modelMetadataLoading, setModelMetadataLoading] = useState(true);
  const [modelMetadataError, setModelMetadataError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    api
      .getFormats()
      .then((f) => {
        if (active) setAvailableFormats(f);
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    let active = true;
    setModelMetadataLoading(true);
    setModelMetadataError(null);
    api
      .getModelMetadata()
      .then((data) => {
        if (active) {
          setModelMetadata(data as ModelMetadataApiResponse);
          setModelMetadataError(null);
        }
      })
      .catch((err) => {
        if (active) {
          setModelMetadata(null);
          setModelMetadataError(err instanceof Error ? err.message : String(err));
        }
      })
      .finally(() => {
        if (active) setModelMetadataLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  const loadAccuracyTrend = useCallback(async () => {
    setTrendError(null);
    setTrendLoading(true);
    try {
      const filters: AccuracyTrendFilters = {
        format: format || undefined,
        start_date: startDate || undefined,
        end_date: endDate || undefined,
        order: 'asc',
        limit: Math.min(Math.max(1, limit), MAX_LIMIT),
        cache: 'read',
        use_unified_model: predictionModel === 'unified',
      };
      const data = await api.accuracyTrend(filters);
      setTrendData(data);
    } catch (e) {
      setTrendError(e instanceof Error ? e.message : String(e));
      setTrendData(null);
    } finally {
      setTrendLoading(false);
    }
  }, [format, startDate, endDate, limit, predictionModel]);

  const handleRegistryFile = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    setRegistryError(null);
    setRegistry(null);
    setRegistryFile(file || null);
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      try {
        const text = reader.result as string;
        const parsed = JSON.parse(text) as WalkForwardRegistry;
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
  };

  return {
    format,
    setFormat,
    predictionModel,
    setPredictionModel,
    startDate,
    setStartDate,
    endDate,
    setEndDate,
    limit,
    setLimit,
    maxLimit: MAX_LIMIT,
    availableFormats,
    trendLoading,
    trendError,
    trendData,
    loadAccuracyTrend,
    registryFile,
    registryError,
    registry,
    handleRegistryFile,
    modelMetadata,
    modelMetadataLoading,
    modelMetadataError,
  };
}
