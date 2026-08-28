import { describe, it, expect } from 'vitest';
import {
  MISSING,
  formatBytes,
  formatCount,
  formatDecimal,
  formatDuration,
  formatEpochSeconds,
  formatPercent,
  formatRate,
  formatWhen,
  shortDigest,
} from './format';

/**
 * The point of these is the *shared* rule, not the individual outputs: before this
 * module, absence rendered as `—`, `-`, `''`, `N/A` or `0 B` depending on the panel,
 * so "no data" and "zero data" were indistinguishable in one place and confusingly
 * different in another.
 */
describe('the missing-value rule', () => {
  const formatters = {
    formatBytes,
    formatRate,
    formatCount,
    formatDecimal,
    formatPercent,
    formatDuration,
    formatEpochSeconds,
  };

  it.each(Object.entries(formatters))('%s renders absence as the one marker', (_name, fn) => {
    expect(fn(null)).toBe(MISSING);
    expect(fn(undefined)).toBe(MISSING);
    expect(fn(NaN)).toBe(MISSING);
    expect(fn(Infinity)).toBe(MISSING);
  });

  it('treats zero as a value rather than an absence', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatCount(0)).toBe('0');
    expect(formatDecimal(0)).toBe('0.00');
    expect(formatPercent(0)).toBe('0.0%');
    expect(formatDuration(0)).toBe('0s');
  });

  it('treats a meaningless negative as absent', () => {
    expect(formatBytes(-1)).toBe(MISSING);
    expect(formatDuration(-1)).toBe(MISSING);
  });
});

describe('formatBytes', () => {
  it('scales and keeps one decimal only below ten in a unit', () => {
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(1536)).toBe('1.5 KB');
    expect(formatBytes(20 * 1024)).toBe('20 KB');
    expect(formatBytes(5 * 1024 ** 3)).toBe('5.0 GB');
  });
});

describe('formatDuration', () => {
  it('drops units that would read as zero', () => {
    expect(formatDuration(45)).toBe('45s');
    expect(formatDuration(130)).toBe('2m 10s');
    expect(formatDuration(120)).toBe('2m');
    expect(formatDuration(3900)).toBe('1h 5m');
  });
});

describe('formatWhen', () => {
  it('returns an unparseable value as given rather than hiding it', () => {
    expect(formatWhen('not a date')).toBe('not a date');
    expect(formatWhen('')).toBe(MISSING);
    expect(formatWhen(undefined)).toBe(MISSING);
  });
});

describe('formatRate', () => {
  it('is a size per second', () => {
    expect(formatRate(1536)).toBe('1.5 KB/s');
    expect(formatRate(0)).toBe(MISSING);
  });
});

describe('shortDigest', () => {
  it('abbreviates to twelve characters and leaves shorter values alone', () => {
    expect(shortDigest('a'.repeat(64))).toBe('a'.repeat(12));
    expect(shortDigest('abc')).toBe('abc');
    expect(shortDigest(undefined)).toBe(MISSING);
  });
});
