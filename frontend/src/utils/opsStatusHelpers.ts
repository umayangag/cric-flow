import type { DatasetStatus } from '../types';

/** Format code (e.g. TEST, ODI, T20, T20I). Canonical list is fetched from API via useCanonicalFormats(). */
export type FormatCode = string;

export function asObj(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' ? (v as Record<string, unknown>) : {};
}

export function getFormats(section: unknown): Record<string, unknown> {
  const obj = asObj(section);
  return asObj((obj as { formats?: unknown }).formats);
}

export function readStatus(v: unknown): 'ok' | 'stale' | 'missing' | 'unknown' {
  const s = typeof v === 'string' ? v : undefined;
  if (s === 'ok' || s === 'stale' || s === 'missing' || s === 'unknown') return s;
  return 'unknown';
}

export function readNumber(v: unknown): number | undefined {
  if (typeof v === 'number' && isFinite(v)) return v;
  return undefined;
}

// Minimal, forward-compatible Ops Status contract.
type ServicesStatus = {
  api_health?: boolean;
  api_readiness?: boolean;
  ml_health?: boolean;
};

/** One run directory, as ml-service reports it and go-app copies it through (H-16). */
export type OpsRun = {
  run_id?: string;
  created_at?: string;
  cutoff?: string;
  git_sha?: string;
  dataset_sha?: string;
  formats?: string[];
  has_manifest?: boolean;
  current?: boolean;
  loaded?: boolean;
};

/** The artifacts section: runs rather than a formats-by-model-kind matrix. */
export type OpsArtifacts = {
  root?: string;
  reachable?: boolean;
  current_run?: string | null;
  loaded_run?: string | null;
  ratings_through?: string | null;
  ratings?: { fresh?: boolean; age_days?: number | null; max_age_days?: number } | null;
  /** Why nothing is loaded, when a run on disk was refused (D-6). */
  error?: string | null;
  runs?: OpsRun[];
};

export type OpsStatus = {
  timestamp: string;
  services?: ServicesStatus;
  db?: unknown;
  dataset?: DatasetStatus;
  artifacts?: OpsArtifacts;
  pipeline?: { steps?: Record<string, { running?: boolean }> };
  fielding?: unknown;
  suggestions?: Array<{ reason: string; commands: string[] }>;
  [key: string]: unknown;
};
