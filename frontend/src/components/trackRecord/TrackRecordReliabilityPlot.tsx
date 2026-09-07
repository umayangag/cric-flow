import React from 'react';
import { Box, Typography } from '@mui/material';
import type { TrackRecordReliabilityBin } from '../../types';
import { MetricLabel } from '../common/MetricInfo';

type Props = {
  bins: TrackRecordReliabilityBin[];
  binCount: number;
  n: number;
};

const SIZE = 260;
const PAD = 36;

function scale(value: number): number {
  return PAD + value * (SIZE - 2 * PAD);
}

/**
 * The reliability curve as the harness draws it: mean predicted against observed
 * frequency, one point per equal-width bin, the diagonal for reference and n on every
 * point. Bins with no predictions are not drawn and nothing joins the points: with tens
 * of predictions a line through them would look like a curve the record has not earned.
 * An empty record draws the axes and says so.
 */
const TrackRecordReliabilityPlot: React.FC<Props> = ({ bins, binCount, n }) => (
  <Box>
    <Typography variant="subtitle2" fontWeight={600} gutterBottom>
      <MetricLabel
        metricKey="reliability"
        label="Reliability"
        value={`${n} scored predictions in ${binCount} bins`}
      />
    </Typography>
    <svg
      width={SIZE}
      height={SIZE}
      viewBox={`0 0 ${SIZE} ${SIZE}`}
      role="img"
      aria-label={`reliability plot, ${bins.length} of ${binCount} bins populated, n ${n}`}
      data-testid="reliability-plot"
    >
      <line
        x1={scale(0)}
        y1={scale(1)}
        x2={scale(1)}
        y2={scale(1)}
        stroke="currentColor"
        strokeOpacity={0.4}
      />
      <line
        x1={scale(0)}
        y1={scale(0)}
        x2={scale(0)}
        y2={scale(1)}
        stroke="currentColor"
        strokeOpacity={0.4}
      />
      <line
        x1={scale(0)}
        y1={scale(1)}
        x2={scale(1)}
        y2={scale(0)}
        stroke="currentColor"
        strokeOpacity={0.25}
        strokeDasharray="4 4"
      />
      {Array.from({ length: binCount + 1 }, (_, k) => k / binCount).map((tick) => (
        <line
          key={tick}
          x1={scale(tick)}
          y1={scale(1)}
          x2={scale(tick)}
          y2={scale(1) + 4}
          stroke="currentColor"
          strokeOpacity={0.4}
        />
      ))}
      <text x={scale(0.5)} y={SIZE - 6} textAnchor="middle" fontSize={11} fill="currentColor">
        predicted P(team1 wins)
      </text>
      <text
        x={10}
        y={scale(0.5)}
        textAnchor="middle"
        fontSize={11}
        fill="currentColor"
        transform={`rotate(-90 10 ${scale(0.5)})`}
      >
        observed
      </text>
      {bins.map((bin) => (
        <g key={bin.lo} data-testid="reliability-point">
          <circle
            cx={scale(bin.predicted)}
            cy={scale(1 - bin.observed)}
            r={4}
            fill="currentColor"
          />
          <text
            x={scale(bin.predicted) + 7}
            y={scale(1 - bin.observed) - 6}
            fontSize={10}
            fill="currentColor"
          >
            n={bin.n}
          </text>
        </g>
      ))}
      {bins.length === 0 && (
        <text
          x={scale(0.5)}
          y={scale(0.5)}
          textAnchor="middle"
          fontSize={12}
          fill="currentColor"
          opacity={0.7}
        >
          no scored predictions yet
        </text>
      )}
    </svg>
  </Box>
);

export default TrackRecordReliabilityPlot;
