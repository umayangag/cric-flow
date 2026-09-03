import React from 'react';
import { Box } from '@mui/material';
import { useMetricGlossary } from '../../context/MetricGlossaryContext';
import { formatShare, formatStat, statMean, type ReportNumber } from '../../utils/evaluationReport';
import { baselineScore, metricPaint, metricScore, scoreLabel } from '../../utils/metricColor';

type MetricValueProps = {
  /** The glossary key this number is reported under; it carries the anchors and the direction. */
  metricKey: string;
  value: ReportNumber;
  /**
   * The same metric on the baseline printed beside it, for a number in the target's own
   * units — pinball, MAE, a hit rate — where no absolute anchor exists. Ignored when the
   * glossary gives the metric anchors of its own, which are the harness's and not a ratio.
   */
  baseline?: ReportNumber;
  /** How the number is written: a plain statistic, or a percentage. */
  as?: 'stat' | 'share';
  digits?: number;
  /** What to render instead of the formatted number, where the surface prints more than it. */
  text?: string;
  /**
   * `chip` tints the number's own background, which is what a table of numbers needs;
   * `text` colours the digits alone, for a headline already sitting in its own tile.
   */
  variant?: 'chip' | 'text';
};

/**
 * One evaluation number, painted red-to-green against the band the glossary states.
 *
 * Every surface that shows a measured number renders this, so the reading of "is 0.786
 * coverage good?" is made once, from the served anchors, rather than by each table
 * deciding for itself. A metric the glossary gives no anchors — a width that means
 * nothing without its coverage — renders as plain text, deliberately.
 */
const MetricValue: React.FC<MetricValueProps> = ({
  metricKey,
  value,
  baseline,
  as = 'stat',
  digits,
  text,
  variant = 'chip',
}) => {
  const entry = useMetricGlossary()(metricKey);
  const formatted =
    text ?? (as === 'share' ? formatShare(value, digits ?? 1) : formatStat(value, digits ?? 3));

  const mean = statMean(value);
  const absolute = metricScore(mean, entry);
  const score = absolute ?? baselineScore(mean, statMean(baseline), entry?.direction);

  if (score == null) return <>{formatted}</>;

  const paint = metricPaint(score);
  const label = `${formatted}, ${scoreLabel(score)}`;

  if (variant === 'text') {
    return (
      <Box component="span" aria-label={label} title={label} sx={{ color: paint.color }}>
        {formatted}
      </Box>
    );
  }

  return (
    <Box
      component="span"
      aria-label={label}
      title={label}
      sx={{
        ...paint,
        display: 'inline-block',
        px: 0.75,
        py: 0.25,
        borderRadius: 1,
        fontWeight: 600,
        fontVariantNumeric: 'tabular-nums',
        whiteSpace: 'nowrap',
      }}
    >
      {formatted}
    </Box>
  );
};

export default MetricValue;
