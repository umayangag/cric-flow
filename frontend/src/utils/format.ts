/**
 * Every formatter this app renders a value through.
 *
 * They were spread across at least eight components, and the duplicates did not
 * agree. `formatBytes` existed three times: one answered `—` for zero, one `0 B`, one
 * `—` for anything non-finite. Absence was rendered as `—`, `-`, `''`, `N/A` or `0 B`
 * depending on which panel you were looking at, which makes "no data" and "zero data"
 * indistinguishable in one place and identical-looking-but-different in another.
 *
 * The rule, applied by all of them:
 *
 *   - **absent** (null, undefined, NaN, Infinity) → {@link MISSING}
 *   - **zero** is a value, not an absence — `0 B`, `0`, `0.00`
 *   - a **negative** where negatives are meaningless (sizes, durations) is absent
 */

/** What every formatter renders when there is nothing to render. */
export const MISSING = '—';

/** True for a number that can actually be displayed. */
function isReal(n: number | null | undefined): n is number {
  return typeof n === 'number' && Number.isFinite(n);
}

/**
 * Bytes as a short human-readable size: `4.2 MB`, `812 KB`, `0 B`.
 *
 * One decimal below 10 in a unit, none above, because "1024 KB" and "1.0 MB" are the
 * same size and only one of them reads like a size.
 */
export function formatBytes(bytes: number | null | undefined): string {
  if (!isReal(bytes) || bytes < 0) return MISSING;
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / 1024 ** exponent;
  return `${value >= 10 || exponent === 0 ? Math.round(value) : value.toFixed(1)} ${units[exponent]}`;
}

/** Bytes per second as a short rate, e.g. `4.2 MB/s`. */
export function formatRate(bytesPerSec: number | null | undefined): string {
  if (!isReal(bytesPerSec) || bytesPerSec <= 0) return MISSING;
  return `${formatBytes(bytesPerSec)}/s`;
}

/** A whole number with thousands separators, e.g. `21,253`. */
export function formatCount(n: number | null | undefined): string {
  if (!isReal(n)) return MISSING;
  return Math.round(n).toLocaleString();
}

/** A number to a fixed number of decimals, e.g. `24.13`. */
export function formatDecimal(n: number | null | undefined, decimals = 2): string {
  if (!isReal(n)) return MISSING;
  return n.toFixed(decimals);
}

/**
 * A fraction as a percentage, e.g. `0.732` → `73.2%`.
 *
 * It takes a fraction rather than an already-multiplied number because every caller
 * in this app holds a fraction, and a formatter that accepts both is one that gets
 * called wrongly.
 */
export function formatPercent(fraction: number | null | undefined, decimals = 1): string {
  if (!isReal(fraction)) return MISSING;
  return `${(fraction * 100).toFixed(decimals)}%`;
}

/**
 * An RFC3339 timestamp as local time.
 *
 * An unparseable value is returned as given rather than as {@link MISSING}: the
 * server sent *something*, and showing it is more useful than hiding it behind a dash
 * that reads as "nothing was sent".
 */
/**
 * A probability in percentage points, which is how the selection surfaces show a marginal
 * value or a gap between two elevens: `0.023` → `2.3 pp`.
 *
 * Unsigned, unlike `formatProbabilityChange`: these are magnitudes of an explanation, not
 * a movement between two answers, and a `+` in front of one would read as a change.
 */
export function formatProbabilityPoints(fraction: number | null | undefined, decimals = 1): string {
  if (!isReal(fraction)) return MISSING;
  return `${(fraction * 100).toFixed(decimals)} pp`;
}

/**
 * A point with the band the model gave it: `23 (4–55)`.
 *
 * The range is never optional presentation. A median printed alone reads as a promise the
 * model never made, so where a range exists it is shown, and where none does the bare
 * point is shown rather than an invented interval.
 */
export function pointWithRange(
  value: number | null | undefined,
  range: { p10: number; p90: number } | null | undefined,
  digits = 0,
): string {
  if (!isReal(value)) return MISSING;
  const point = value.toFixed(digits);
  if (!range) return point;
  return `${point} (${range.p10.toFixed(digits)}–${range.p90.toFixed(digits)})`;
}

export function formatWhen(value: string | null | undefined): string {
  if (!value) return MISSING;
  const d = new Date(value);
  return isNaN(d.getTime()) ? value : d.toLocaleString();
}

/** A Unix timestamp in seconds as local time. */
export function formatEpochSeconds(seconds: number | null | undefined): string {
  if (!isReal(seconds)) return MISSING;
  const d = new Date(seconds * 1000);
  return isNaN(d.getTime()) ? MISSING : d.toLocaleString();
}

/** Seconds as a compact duration: `1h 5m`, `2m 10s`, `45s`, `0s`. */
export function formatDuration(seconds: number | null | undefined): string {
  if (!isReal(seconds) || seconds < 0) return MISSING;
  const total = Math.floor(seconds);
  if (total < 60) return `${total}s`;
  const minutes = Math.floor(total / 60);
  if (minutes >= 60) return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
  const rest = total % 60;
  return rest > 0 ? `${minutes}m ${rest}s` : `${minutes}m`;
}

/**
 * A metric value as a run reports it: integers with separators, fractions to four
 * places.
 *
 * Four places rather than two because these are model metrics — an RMSE that moved
 * from 24.1312 to 24.1319 between runs is a metric that did not move, and rounding
 * that to `24.13` twice would hide which of the two it was.
 */
export function formatMetricValue(value: number | null | undefined): string {
  if (!isReal(value)) return MISSING;
  return Number.isInteger(value) ? value.toLocaleString() : value.toFixed(4);
}

/**
 * A SHA-256 as its first 12 characters.
 *
 * Enough to tell two datasets apart at a glance, short enough to sit in a table cell.
 * The full value stays available as a tooltip wherever this is used — abbreviating
 * without keeping the original is how a digest stops being verifiable.
 */
export function shortDigest(sha256: string | null | undefined): string {
  if (!sha256) return MISSING;
  return sha256.length <= 12 ? sha256 : sha256.slice(0, 12);
}
