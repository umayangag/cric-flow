/**
 * Helpers to derive UI data from GET /api/ml/model-metadata response.
 * Keeps Workbench in sync with backend model kinds, levels, and model_modes.
 */
import type { ModelMetadataApiResponse, ModelMetadataEntry, ModelModeEntry } from '../types';

const TRAINABLE_ORDER = ['batting', 'bowling', 'fielding', 'extras', 'win'];
export const DISPLAY_ORDER = [
  'batting',
  'bowling',
  'fielding',
  'extras',
  'win',
  'combination_meta',
];

function isModelEntry(v: unknown): v is ModelMetadataEntry {
  return (
    v != null &&
    typeof v === 'object' &&
    'features' in v &&
    Array.isArray((v as ModelMetadataEntry).features) &&
    'level' in v
  );
}

/** Extract model-kind entries only (exclude model_modes). */
export function getModelEntries(
  api: ModelMetadataApiResponse | null,
): Record<string, ModelMetadataEntry> {
  if (!api) return {};
  const out: Record<string, ModelMetadataEntry> = {};
  for (const key of Object.keys(api)) {
    if (key === 'model_modes') continue;
    const val = api[key];
    if (isModelEntry(val)) out[key] = val;
  }
  return out;
}

/** Keys for "Train (...)" in pipeline summary (batting, bowling, fielding, extras, win). */
export function getTrainableModelKeys(api: ModelMetadataApiResponse | null): string[] {
  const entries = getModelEntries(api);
  return TRAINABLE_ORDER.filter((k) => k in entries);
}

export function hasCombinationMeta(api: ModelMetadataApiResponse | null): boolean {
  const entries = getModelEntries(api);
  return 'combination_meta' in entries;
}

/** Player-level model keys for flow diagram (level === 'player'). */
export function getPlayerLevelKeys(entries: Record<string, ModelMetadataEntry>): string[] {
  return DISPLAY_ORDER.filter((k) => entries[k]?.level === 'player');
}

/** Match-level model keys for flow diagram (level === 'match'). */
export function getMatchLevelKeys(entries: Record<string, ModelMetadataEntry>): string[] {
  return DISPLAY_ORDER.filter((k) => entries[k]?.level === 'match');
}

/** model_modes from API (legacy, per_format) for Prediction model selector. */
export function getModelModes(api: ModelMetadataApiResponse | null): ModelModeEntry[] {
  const modes = api?.model_modes;
  return Array.isArray(modes) ? modes : [];
}
