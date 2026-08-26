/**
 * Formatting shared by the Data tab.
 *
 * These live outside the components because the registry table, the extract picker
 * and the progress card all render the same values, and a digest abbreviated three
 * different ways reads as three different digests.
 */

/** An RFC3339 timestamp as local time, or an em dash when absent. */
export function formatWhen(value?: string): string {
  if (!value) return '—';
  const d = new Date(value);
  return isNaN(d.getTime()) ? value : d.toLocaleString();
}

/**
 * A SHA-256 as its first 12 characters.
 *
 * Enough to tell two datasets apart at a glance, short enough to sit in a table cell.
 * The full value stays available as a tooltip wherever this is used — abbreviating
 * without keeping the original is how a digest stops being verifiable.
 */
export function shortDigest(sha256?: string): string {
  if (!sha256) return '—';
  return sha256.length <= 12 ? sha256 : sha256.slice(0, 12);
}

/** Bytes per second as a short rate, e.g. "4.2 MB/s". */
export function formatRate(bytesPerSec?: number): string {
  if (!bytesPerSec || bytesPerSec <= 0) return '—';
  const units = ['B/s', 'KB/s', 'MB/s', 'GB/s'];
  const exponent = Math.min(Math.floor(Math.log(bytesPerSec) / Math.log(1024)), units.length - 1);
  const value = bytesPerSec / 1024 ** exponent;
  return `${value >= 10 || exponent === 0 ? Math.round(value) : value.toFixed(1)} ${units[exponent]}`;
}
