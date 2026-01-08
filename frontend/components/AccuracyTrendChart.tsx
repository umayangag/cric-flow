import React from 'react';

type ProgressivePoint = Record<string, number>;

export interface AccuracyTrendChartProps {
  data: ProgressivePoint[];
  width?: number;
  height?: number;
}

// A lightweight SVG line chart for the progressive averages series.
// Draws lines for keys if present: player_runs_mae_avg, team_runs_mae_avg, team_winner_accuracy_avg (mapped to 0..1 scale on right axis).
export const AccuracyTrendChart: React.FC<AccuracyTrendChartProps> = ({ data, width = 680, height = 260 }) => {
  if (!data || data.length === 0) {
    return <div style={{ padding: 8, color: '#666' }}>No progressive data to display.</div>;
  }

  const padding = { top: 20, right: 48, bottom: 24, left: 40 };
  const innerW = width - padding.left - padding.right;
  const innerH = height - padding.top - padding.bottom;

  const nValues = data.map(d => d.n ?? 0);
  const maxN = nValues.length > 0 ? Math.max(...nValues) : 1;

  const keysLeft = ['player_runs_mae_avg', 'team_runs_mae_avg'];
  const keyRight = 'team_winner_accuracy_avg';

  // Compute Y domain for left axis
  const leftValues: number[] = [];
  data.forEach(d => {
    keysLeft.forEach(k => {
      if (typeof d[k] === 'number') leftValues.push(d[k]);
    });
  });
  const maxLeft = leftValues.length ? Math.max(...leftValues) : 1;

  const x = (n: number) => padding.left + (n - 1) / Math.max(1, maxN - 1) * innerW;
  const yLeft = (v: number) => padding.top + (1 - v / Math.max(1e-9, maxLeft)) * innerH;
  const yRight = (v: number) => padding.top + (1 - v) * innerH; // 0..1

  const makePath = (key: string, useRightAxis = false) => {
    const pts = data
      .filter(d => typeof d[key] === 'number' && typeof d.n === 'number')
      .map(d => [x(d.n), useRightAxis ? yRight(d[key]) : yLeft(d[key])] as [number, number]);
    if (pts.length === 0) return '';
    return pts.map((p, i) => (i === 0 ? `M ${p[0]} ${p[1]}` : `L ${p[0]} ${p[1]}`)).join(' ');
  };

  const gridY = 4;
  const gridLines = Array.from({ length: gridY + 1 }, (_, i) => padding.top + (i / gridY) * innerH);

  return (
    <svg width={width} height={height} role="img" aria-label="Accuracy trend progressive averages chart">
      {/* background */}
      <rect x={0} y={0} width={width} height={height} fill="#fff" />

      {/* grid */}
      {gridLines.map((y, i) => (
        <line key={i} x1={padding.left} y1={y} x2={width - padding.right} y2={y} stroke="#eee" />
      ))}

      {/* axes labels */}
      <text x={padding.left} y={padding.top - 6} fill="#333" fontSize={12}>MAE (left)</text>
      <text x={width - padding.right} y={padding.top - 6} fill="#333" fontSize={12} textAnchor="end">Accuracy (right)</text>

      {/* lines */}
      {/* player_runs_mae_avg */}
      {leftValues.length > 0 && (
        <path d={makePath('player_runs_mae_avg')} fill="none" stroke="#1f77b4" strokeWidth={2} />
      )}
      {/* team_runs_mae_avg */}
      {leftValues.length > 0 && (
        <path d={makePath('team_runs_mae_avg')} fill="none" stroke="#2ca02c" strokeWidth={2} />
      )}
      {/* team_winner_accuracy_avg (right axis) */}
      {data.some(d => typeof d[keyRight] === 'number') && (
        <path d={makePath(keyRight, true)} fill="none" stroke="#d62728" strokeWidth={2} />
      )}

      {/* legend */}
      <g transform={`translate(${padding.left}, ${height - padding.bottom + 6})`}>
        <circle cx={0} cy={0} r={4} fill="#1f77b4" />
        <text x={10} y={4} fontSize={12}>player_runs_mae_avg</text>
        <circle cx={160} cy={0} r={4} fill="#2ca02c" />
        <text x={170} y={4} fontSize={12}>team_runs_mae_avg</text>
        <circle cx={300} cy={0} r={4} fill="#d62728" />
        <text x={310} y={4} fontSize={12}>team_winner_accuracy_avg</text>
      </g>
    </svg>
  );
};

export default AccuracyTrendChart;
