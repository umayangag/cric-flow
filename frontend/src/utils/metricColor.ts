import type { MetricGlossaryEntry } from '../types';

/**
 * Where a reported number sits between "as bad as this metric gets" and "as good", and
 * what colour that is.
 *
 * The judgement is never made here. The anchors come from the served glossary, beside the
 * code that computes the metric, so a value cannot be painted green while the explainer
 * next to it says the opposite. What lives here is only the arithmetic: one score in
 * `[0, 1]`, and one ramp from it to a colour. A metric the glossary gives no anchors for
 * scores `null` and is painted nothing — an uncoloured number is the honest answer, and a
 * grey one invented here would be a claim the harness never measured.
 */

/** 0 is the red end of the metric's reference band, 1 the green end. */
export type MetricScore = number;

const clamp = (value: number): MetricScore => Math.min(1, Math.max(0, value));

/**
 * A relative improvement of this much over a baseline saturates the ramp. It is a display
 * convention, not a measurement: it exists so metrics in the target's own units — pinball,
 * MAE, hit rate — can still be read against the baseline printed beside them.
 */
const BASELINE_SATURATION = 0.2;

export function metricScore(
  value: number | null | undefined,
  entry: MetricGlossaryEntry | undefined,
): MetricScore | null {
  if (value == null || !Number.isFinite(value)) return null;
  const scale = entry?.scale;
  if (!scale) return null;
  const { bad, good } = scale;

  switch (entry?.direction) {
    case 'higher':
    case 'lower':
      // One formula covers both: `bad` sits below `good` for one and above it for the other.
      return good === bad ? null : clamp((value - bad) / (good - bad));
    case 'nominal':
      // Read by distance from the nominal value, in either direction.
      return good === bad ? null : clamp(1 - Math.abs(value - good) / Math.abs(bad - good));
    case 'exact':
      return value === good ? 1 : 0;
    default:
      return null;
  }
}

/**
 * The same score for a metric with no absolute anchor, read against the baseline the
 * surface prints beside it: the model's pinball loss against the career quantiles', say.
 * Equal to the baseline is the middle of the ramp, because matching a baseline is neither
 * progress nor a regression.
 */
export function baselineScore(
  value: number | null | undefined,
  baseline: number | null | undefined,
  direction: MetricGlossaryEntry['direction'] | undefined,
): MetricScore | null {
  if (value == null || baseline == null) return null;
  if (!Number.isFinite(value) || !Number.isFinite(baseline) || baseline === 0) return null;
  if (direction !== 'higher' && direction !== 'lower') return null;

  const gain = direction === 'higher' ? value - baseline : baseline - value;
  return clamp(0.5 + gain / Math.abs(baseline) / (2 * BASELINE_SATURATION));
}

/** Red → amber → green: the fill a value is tinted with. */
const FILL_RAMP = ['#ef4444', '#f59e0b', '#22c55e'];
/** The same ramp darkened, so the number itself stays legible on its tint. */
const INK_RAMP = ['#991b1b', '#92400e', '#166534'];

function channels(hex: string): [number, number, number] {
  const value = parseInt(hex.slice(1), 16);
  return [(value >> 16) & 255, (value >> 8) & 255, value & 255];
}

/** The colour a score lands on, interpolated between the ramp's stops. */
function rampColor(ramp: string[], score: MetricScore): string {
  const position = clamp(score) * (ramp.length - 1);
  const lower = Math.floor(position);
  const upper = Math.min(ramp.length - 1, lower + 1);
  const between = position - lower;
  const from = channels(ramp[lower]);
  const to = channels(ramp[upper]);
  const mixed = from.map((channel, i) => Math.round(channel + (to[i] - channel) * between));
  return `rgb(${mixed.join(', ')})`;
}

/** The ramp as one CSS gradient, so a legend cannot show colours the values do not use. */
export const metricSpectrumGradient = `linear-gradient(90deg, ${FILL_RAMP.join(', ')})`;

export type MetricPaint = {
  /** The tint behind the number, faint enough that a table of them stays readable. */
  backgroundColor: string;
  /** The number's own colour. */
  color: string;
};

export function metricPaint(score: MetricScore): MetricPaint {
  const fill = rampColor(FILL_RAMP, score);
  return {
    backgroundColor: fill.replace('rgb(', 'rgba(').replace(')', ', 0.18)'),
    color: rampColor(INK_RAMP, score),
  };
}

/**
 * The same verdict in words, for the screen reader and the tooltip: colour alone must not
 * be the only thing that says whether a number is good.
 */
export function scoreLabel(score: MetricScore): string {
  if (score >= 0.8) return 'strong against its reference band';
  if (score >= 0.6) return 'good against its reference band';
  if (score >= 0.4) return 'middling against its reference band';
  if (score >= 0.2) return 'weak against its reference band';
  return 'poor against its reference band';
}
