export const FORMATS = ['TEST', 'ODI', 'T20I', 'T20'] as const;
export type FormatCode = (typeof FORMATS)[number];

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

type PrecomputeFormats = Record<
  string,
  { status?: 'ok' | 'stale' | 'missing' | string } | undefined
>;

export type ExportFile = { name?: string; exists?: boolean };
type ExportFormats = Record<string, { files?: ExportFile[] } | undefined>;

type ArtifactUnit = { exists?: boolean; loaded?: boolean };
type ArtifactFormats = Record<
  string,
  { batting?: ArtifactUnit; bowling?: ArtifactUnit } | undefined
>;

export type OpsStatus = {
  timestamp: string;
  services?: ServicesStatus;
  db?: unknown;
  precompute?: { formats?: PrecomputeFormats };
  exports?: { formats?: ExportFormats };
  artifacts?: { formats?: ArtifactFormats };
  pipeline?: { steps?: Record<string, { running?: boolean }> };
  fielding?: unknown;
  weather?: unknown;
  suggestions?: Array<{ reason: string; commands: string[] }>;
  [key: string]: unknown;
};
