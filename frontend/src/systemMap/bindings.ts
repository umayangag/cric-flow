import type { SystemMapBinding, ValueKind } from './types';

/**
 * Reading a live value off the endpoints the app already calls.
 *
 * The rule the map is built on is that no number is written into it. A node names a key
 * and a path; this resolves the path against /ops/status, /api/ml/xi-status or the
 * evaluation report, so the figure on the map is the figure those endpoints returned —
 * the same object the Ops and Evaluation tabs render. A path that resolves to nothing is
 * a dash, never a guess and never a crash: the report is absent until `make evaluate`
 * has run, and a manifest field can be missing on an older run.
 */

/** What a value reads as when the endpoint did not carry it. */
export const MISSING = '—';

export type BindingSources = {
  ops_status: unknown;
  xi_status: unknown;
  report: unknown;
};

export const emptySources: BindingSources = { ops_status: null, xi_status: null, report: null };

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

/**
 * Follow a dotted path, with one extra form: `list[field=value]` picks the entry of an
 * array whose `field` equals `value`. /ops/status reports per-table row counts as a list
 * of `{table_name, row_count}`, and naming the table is clearer than naming an index.
 */
export function resolvePath(root: unknown, path: string): unknown {
  let cursor: unknown = root;
  for (const rawSegment of path.split('.')) {
    if (cursor === null || cursor === undefined) return undefined;
    const selector = /^([^[]+)\[([^\]=]+)=([^\]]+)\]$/.exec(rawSegment);
    if (selector) {
      const list = isRecord(cursor) ? cursor[selector[1]] : undefined;
      if (!Array.isArray(list)) return undefined;
      cursor = list.find((entry) => isRecord(entry) && String(entry[selector[2]]) === selector[3]);
      continue;
    }
    if (!isRecord(cursor)) return undefined;
    cursor = cursor[rawSegment];
  }
  return cursor;
}

/**
 * The harness reports a walk-forward figure as `{mean, sd, n_folds}` and a locked-window
 * figure as a bare number. Both are the same measurement, so both read as one here and
 * the map does not have to know which shape a path lands on.
 */
export function unwrapFoldStat(value: unknown): unknown {
  if (isRecord(value) && typeof value.mean === 'number') return value.mean;
  return value;
}

function formatBytes(bytes: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = bytes;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${unit === 0 ? size : size.toFixed(1)} ${units[unit]}`;
}

export function formatValue(value: unknown, kind: ValueKind): string {
  const resolved = unwrapFoldStat(value);
  if (resolved === null || resolved === undefined || resolved === '') return MISSING;

  switch (kind) {
    case 'integer':
      return typeof resolved === 'number'
        ? Math.round(resolved).toLocaleString()
        : String(resolved);
    case 'number1':
      return typeof resolved === 'number' ? resolved.toFixed(1) : String(resolved);
    case 'ratio3':
      return typeof resolved === 'number' ? resolved.toFixed(3) : String(resolved);
    case 'signed3':
      return typeof resolved === 'number'
        ? `${resolved >= 0 ? '+' : ''}${resolved.toFixed(3)}`
        : String(resolved);
    case 'percent':
      return typeof resolved === 'number' ? `${(resolved * 100).toFixed(1)} %` : String(resolved);
    case 'bytes':
      return typeof resolved === 'number' ? formatBytes(resolved) : String(resolved);
    case 'yes_no':
      return resolved === true ? 'yes' : resolved === false ? 'no' : String(resolved);
    case 'list':
      return Array.isArray(resolved)
        ? resolved.length
          ? resolved.join(', ')
          : MISSING
        : String(resolved);
    case 'sha':
      return typeof resolved === 'string' && resolved.length > 12
        ? `${resolved.slice(0, 12)}…`
        : String(resolved);
    case 'date':
      return String(resolved).slice(0, 10);
    case 'timestamp':
      return String(resolved).replace('T', ' ').replace(/\.\d+/, '').replace('Z', ' UTC');
    case 'text':
    default:
      return String(resolved);
  }
}

/** One rendered live value: what to call it, what it says, and whether it is really there. */
export type ResolvedValue = {
  key: string;
  label: string;
  /** The format this row is for, when the binding is per format. */
  format?: string;
  text: string;
  present: boolean;
  glossaryKey?: string;
};

/** The formats the evaluation report actually holds, in the order it holds them. */
export function reportFormats(report: unknown): string[] {
  const formats = resolvePath(report, 'formats');
  return isRecord(formats) ? Object.keys(formats) : [];
}

/**
 * Resolve one binding into the rows the detail panel shows: one row, or one row per
 * format when the path names `{format}`.
 */
export function resolveBinding(
  binding: SystemMapBinding,
  sources: BindingSources,
): ResolvedValue[] {
  const root = sources[binding.source];

  if (!binding.per_format) {
    const raw = resolvePath(root, binding.path);
    const text = formatValue(raw, binding.value_kind);
    return [
      {
        key: binding.key,
        label: binding.label,
        text,
        present: text !== MISSING,
        glossaryKey: binding.glossary_key,
      },
    ];
  }

  const formats = reportFormats(sources.report);
  if (formats.length === 0) {
    return [
      {
        key: binding.key,
        label: binding.label,
        text: MISSING,
        present: false,
        glossaryKey: binding.glossary_key,
      },
    ];
  }
  return formats.map((format) => {
    const raw = resolvePath(root, binding.path.replace('{format}', format));
    const text = formatValue(raw, binding.value_kind);
    return {
      key: `${binding.key}:${format}`,
      label: binding.label,
      format,
      text,
      present: text !== MISSING,
      glossaryKey: binding.glossary_key,
    };
  });
}

// --- The gate registry, read live ---------------------------------------------------

/** One gate as the report carries it (H-23), with the value at the path it declares. */
export type ResolvedGate = {
  id: string;
  name: string;
  varies: string;
  fixed: string;
  decides: string;
  values: Array<{ format: string; text: string }>;
};

/**
 * The harness's gates, taken from the report rather than from the contract.
 *
 * The contract names the gate ids and nothing else. The name, what a gate varies, what it
 * holds fixed, what it decides and *where its number lives* all come from the registry the
 * harness embeds in its own report — so the map cannot describe a gate differently from
 * the code that runs it, and no report path for a gate is typed into the map at all.
 */
export function resolveGates(ids: readonly string[], report: unknown): ResolvedGate[] {
  const registry = resolvePath(report, 'gates.registry');
  const formats = reportFormats(report);

  return ids.map((id) => {
    const entry = isRecord(registry) ? registry[id] : undefined;
    const gate = isRecord(entry) ? entry : {};
    const reportPath = typeof gate.report_path === 'string' ? gate.report_path : null;

    let values: Array<{ format: string; text: string }> = [];
    if (reportPath && reportPath.startsWith('report:')) {
      const raw = resolvePath(report, reportPath.slice('report:'.length));
      values = [{ format: 'all formats', text: formatValue(raw, 'text') }];
    } else if (reportPath) {
      values = formats.map((format) => ({
        format,
        text: formatValue(resolvePath(report, `formats.${format}.${reportPath}`), 'text'),
      }));
    }

    return {
      id,
      name: typeof gate.name === 'string' ? gate.name : id,
      varies: typeof gate.varies === 'string' ? gate.varies : MISSING,
      fixed: typeof gate.fixed === 'string' ? gate.fixed : MISSING,
      decides: typeof gate.decides === 'string' ? gate.decides : MISSING,
      values,
    };
  });
}
