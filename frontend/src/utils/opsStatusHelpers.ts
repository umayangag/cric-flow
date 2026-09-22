import { FRESHNESS_STATUSES, RETRAIN_STATUSES } from '../types';
import type { DatasetStatus, FreshnessStatus, RetrainStatus } from '../types';

/** Format code (e.g. TEST, ODI, T20, T20I). Canonical list is fetched from API via useCanonicalFormats(). */
export type FormatCode = string;

export function asObj(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' ? (v as Record<string, unknown>) : {};
}

export function getFormats(section: unknown): Record<string, unknown> {
  const obj = asObj(section);
  return asObj((obj as { formats?: unknown }).formats);
}

/**
 * The completeness section's status word: has this format been imported at all?
 *
 * It used to be shared with `db_freshness`'s 7/30 buckets and carried their `stale` with
 * it. The buckets are gone (P2-1) and so is the word: freshness is one verdict now, read
 * off the `freshness` object, and this answers a different question.
 */
export function readCompletenessStatus(v: unknown): 'ok' | 'missing' | 'unknown' {
  const s = typeof v === 'string' ? v : undefined;
  if (s === 'ok' || s === 'missing') return s;
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
  // ml-service's own verdict travels inside this section too, because go-app copies
  // /artifacts/status through whole — but no surface reads it from here. Freshness is
  // read off the assembled `freshness` object, which is the one place it is spelled
  // (P2-1), so the fields are deliberately not declared on this type.
  /** Why nothing is loaded, when a run on disk was refused (D-6). */
  error?: string | null;
  runs?: OpsRun[];
};

/**
 * H-11's verdict as ml-service reported it, plus the word that spells it. Every field but
 * `status` is ml-service's own; nothing on this side recomputes an age against a limit,
 * because this side holds no limit (P2-1).
 */
export type ServedFreshness = {
  status: FreshnessStatus;
  fresh: boolean;
  /** Days from `data_through` to today — the quantity ml-service compared to the limit. */
  data_age_days: number | null;
  max_age_days: number | null;
  /** The served run's cutoff: the date its data was built to, and what the age counts from. */
  data_through: string | null;
  /** The last match the run folded in. Shown, never the verdict: between seasons it walks
   * away from today on its own, and no retrain can move it (SERVE-03). */
  ratings_through: string | null;
  code: string | null;
};

/** One format's import lag: a date and a number of days, as facts. `note` says why there
 * is no date, so an empty format and an unreadable database do not look the same. */
export type FormatLag = {
  latest_match_date: string | null;
  age_days: number | null;
  match_count: number;
  note?: string;
};

/** Whether the database holds matches the served run never saw, with the days and the date. */
export type RetrainDue = {
  status: RetrainStatus;
  days_behind: number | null;
  latest_match_date: string | null;
  format?: string;
};

/**
 * The one freshness object, assembled once by go-app and read by every surface that shows
 * freshness (P2-1): the served verdict as the only badge, the database's per-format lag as
 * a fact beside it, and whether a retrain is due.
 */
export type OpsFreshness = {
  served: ServedFreshness;
  database: Record<string, FormatLag>;
  retrain_due: RetrainDue;
};

/** What a surface shows before /ops/status has answered: no verdict, and it says so. */
export const UNKNOWN_FRESHNESS: OpsFreshness = {
  served: {
    status: 'unknown',
    fresh: false,
    data_age_days: null,
    max_age_days: null,
    data_through: null,
    ratings_through: null,
    code: null,
  },
  database: {},
  retrain_due: { status: 'unknown', days_behind: null, latest_match_date: null },
};

/**
 * Read the freshness object off an /ops/status payload.
 *
 * Every surface goes through here, so a payload without the object (an older backend, or
 * a status poll that has not answered yet) reads `unknown` on all three facts rather than
 * each component inventing its own fallback.
 */
export function readFreshness(data: unknown): OpsFreshness {
  const section = asObj(asObj(data).freshness);
  if (Object.keys(section).length === 0) return UNKNOWN_FRESHNESS;
  const served = asObj(section.served);
  const retrain = asObj(section.retrain_due);
  return {
    served: {
      status: readFreshnessStatus(served.status),
      fresh: served.fresh === true,
      data_age_days: readNumber(served.data_age_days) ?? null,
      max_age_days: readNumber(served.max_age_days) ?? null,
      data_through: typeof served.data_through === 'string' ? served.data_through : null,
      ratings_through: typeof served.ratings_through === 'string' ? served.ratings_through : null,
      code: typeof served.code === 'string' ? served.code : null,
    },
    database: readDatabaseLag(section.database),
    retrain_due: {
      status: readRetrainStatus(retrain.status),
      days_behind: readNumber(retrain.days_behind) ?? null,
      latest_match_date:
        typeof retrain.latest_match_date === 'string' ? retrain.latest_match_date : null,
      format: typeof retrain.format === 'string' ? retrain.format : undefined,
    },
  };
}

function readFreshnessStatus(v: unknown): FreshnessStatus {
  const found = FRESHNESS_STATUSES.find((status) => status === v);
  return found ?? 'unknown';
}

function readRetrainStatus(v: unknown): RetrainStatus {
  const found = RETRAIN_STATUSES.find((status) => status === v);
  return found ?? 'unknown';
}

function readDatabaseLag(v: unknown): Record<string, FormatLag> {
  const out: Record<string, FormatLag> = {};
  for (const [format, raw] of Object.entries(asObj(v))) {
    const row = asObj(raw);
    out[format] = {
      latest_match_date: typeof row.latest_match_date === 'string' ? row.latest_match_date : null,
      age_days: readNumber(row.age_days) ?? null,
      match_count: readNumber(row.match_count) ?? 0,
      note: typeof row.note === 'string' ? row.note : undefined,
    };
  }
  return out;
}

/**
 * How many of the reviewed franchise renames the archive has joined back up.
 *
 * `incomplete` is the one to act on: both rows of a rename are in the data and the link
 * between them was never written, so that club is two clubs with two rating histories.
 * That is what an import which stopped before settlement leaves behind, and until
 * IMPORT-07 no surface said so. A rename this dataset stops before is not a fault, so it
 * is counted separately by the backend and not read here.
 */
export type TeamLineage = {
  status: 'ok' | 'incomplete' | 'unknown';
  renamesConfigured: number | null;
  linked: number | null;
  unlinkedRenames: string[];
};

export function readTeamLineage(data: unknown): TeamLineage {
  const section = asObj(asObj(asObj(data).db).team_lineage);
  const status = section.status;
  return {
    status: status === 'ok' || status === 'incomplete' ? status : 'unknown',
    renamesConfigured: readNumber(section.renames_configured) ?? null,
    linked: readNumber(section.linked) ?? null,
    unlinkedRenames: Array.isArray(section.unlinked_renames)
      ? section.unlinked_renames.filter((entry): entry is string => typeof entry === 'string')
      : [],
  };
}

/** The pill a served status is shown as. One mapping, so the badge cannot differ by tab. */
export function freshnessPillState(
  status: FreshnessStatus,
): 'ok' | 'stale' | 'missing' | 'unknown' {
  switch (status) {
    case 'fresh':
      return 'ok';
    case 'stale':
      return 'stale';
    case 'not_loaded':
      return 'missing';
    default:
      return 'unknown';
  }
}

/**
 * The served verdict in one sentence — the words every surface uses for it.
 *
 * It is one function because the Health tab, the Ops badge and the Lab's readiness notice
 * describing the same verdict in three different sentences is how a reader comes to
 * believe they are three different facts.
 */
export function servedFreshnessLabel(served: ServedFreshness): string {
  switch (served.status) {
    case 'fresh':
      return (
        `data built to ${served.data_through} (${served.data_age_days} days ago, ` +
        `limit ${served.max_age_days}); last match ${served.ratings_through}`
      );
    case 'stale':
      return (
        `data built to ${served.data_through} — ${served.data_age_days} days ago, ` +
        `past the limit of ${served.max_age_days}: ${served.code ?? 'refused'}`
      );
    case 'not_loaded':
      return 'no run loaded, so there are no ratings and no date';
    default:
      return 'the ML service did not answer, so the ratings date is unknown';
  }
}

export type OpsStatus = {
  timestamp: string;
  services?: ServicesStatus;
  db?: unknown;
  dataset?: DatasetStatus;
  artifacts?: OpsArtifacts;
  /** The one freshness verdict, assembled once by go-app (P2-1). */
  freshness?: OpsFreshness;
  pipeline?: { steps?: Record<string, { running?: boolean }> };
  fielding?: unknown;
  suggestions?: Array<{ reason: string; commands: string[] }>;
  [key: string]: unknown;
};
