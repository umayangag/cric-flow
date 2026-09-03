import { describe, it, expect } from 'vitest';
import {
  baselineScore,
  metricPaint,
  metricScore,
  metricSpectrumGradient,
  scoreLabel,
} from './metricColor';
import type { MetricGlossaryEntry } from '../types';

const entry = (
  overrides: Partial<MetricGlossaryEntry> & Pick<MetricGlossaryEntry, 'direction'>,
): MetricGlossaryEntry => ({
  key: 'metric',
  name: 'Metric',
  explanation: 'x',
  band: 'x',
  better: 'x',
  ...overrides,
});

const auc = entry({ direction: 'higher', scale: { bad: 0.5, good: 0.75 } });
const brier = entry({ direction: 'lower', scale: { bad: 0.25, good: 0.18 } });
const coverage = entry({ direction: 'nominal', scale: { bad: 0.65, good: 0.8 } });
const parity = entry({ direction: 'exact', scale: { bad: 0, good: 0 } });
const width = entry({ direction: 'none' });

describe('metricScore', () => {
  it('places a higher-is-better value between the band ends', () => {
    expect(metricScore(0.5, auc)).toBe(0);
    expect(metricScore(0.625, auc)).toBeCloseTo(0.5, 6);
    expect(metricScore(0.75, auc)).toBe(1);
  });

  it('reads a lower-is-better value with the same arithmetic', () => {
    expect(metricScore(0.25, brier)).toBe(0);
    expect(metricScore(0.18, brier)).toBe(1);
  });

  it('clamps a value beyond either end of the band', () => {
    expect(metricScore(0.92, auc)).toBe(1);
    expect(metricScore(0.31, auc)).toBe(0);
  });

  it('scores a nominal metric by its distance from the nominal value, either way', () => {
    expect(metricScore(0.8, coverage)).toBe(1);
    expect(metricScore(0.725, coverage)).toBeCloseTo(0.5, 6);
    expect(metricScore(0.875, coverage)).toBeCloseTo(0.5, 6);
  });

  it('gives an exact metric no credit for being close', () => {
    expect(metricScore(0, parity)).toBe(1);
    expect(metricScore(0.0001, parity)).toBe(0);
  });

  it('scores nothing for a metric the glossary gives no band, or no glossary at all', () => {
    expect(metricScore(12.4, width)).toBeNull();
    expect(metricScore(12.4, undefined)).toBeNull();
  });

  it('scores nothing for a missing or non-finite value', () => {
    expect(metricScore(null, auc)).toBeNull();
    expect(metricScore(Number.NaN, auc)).toBeNull();
  });
});

describe('baselineScore', () => {
  const pinball = entry({ direction: 'lower' });

  it('puts a value equal to its baseline in the middle of the ramp', () => {
    expect(baselineScore(3.0, 3.0, pinball.direction)).toBeCloseTo(0.5, 6);
  });

  it('saturates at a fifth better than the baseline, and a fifth worse', () => {
    expect(baselineScore(2.4, 3.0, pinball.direction)).toBe(1);
    expect(baselineScore(3.6, 3.0, pinball.direction)).toBe(0);
  });

  it('reads a higher-is-better metric the other way round', () => {
    expect(baselineScore(0.6, 0.5, 'higher')).toBeCloseTo(1, 6);
    expect(baselineScore(0.4, 0.5, 'higher')).toBeCloseTo(0, 6);
  });

  it('scores nothing without a usable baseline or a direction to read it with', () => {
    expect(baselineScore(3.0, 0, 'lower')).toBeNull();
    expect(baselineScore(3.0, null, 'lower')).toBeNull();
    expect(baselineScore(3.0, 3.0, 'nominal')).toBeNull();
  });
});

describe('metricPaint', () => {
  it('paints the bad end red and the good end green', () => {
    expect(metricPaint(0).color).toBe('rgb(153, 27, 27)');
    expect(metricPaint(1).color).toBe('rgb(22, 101, 52)');
  });

  it('paints the middle amber, between the two', () => {
    expect(metricPaint(0.5).backgroundColor).toBe('rgba(245, 158, 11, 0.18)');
  });

  it('tints the background from the same ramp the legend shows', () => {
    expect(metricSpectrumGradient).toContain('#ef4444');
    expect(metricSpectrumGradient).toContain('#22c55e');
    expect(metricPaint(0).backgroundColor).toBe('rgba(239, 68, 68, 0.18)');
  });
});

describe('scoreLabel', () => {
  it('says in words what the colour says, for a reader who cannot use it', () => {
    expect(scoreLabel(0.95)).toContain('strong');
    expect(scoreLabel(0.5)).toContain('middling');
    expect(scoreLabel(0.05)).toContain('poor');
  });
});
