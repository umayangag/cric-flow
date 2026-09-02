import React from 'react';
import {
  Box,
  Chip,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import { formatBytes } from '../utils/format';
import { formatCount, formatMetricValue, formatWhen, shortDigest } from '../utils/format';
import { MetricLabel } from './common/MetricInfo';
import { useMetricGlossary } from '../context/MetricGlossaryContext';
import { compareMetrics, type MetricComparison } from '../utils/runMetadata';
import type { Migration, RunMetadata } from '../types';

type Props = {
  metadata: RunMetadata;
  /** Metrics of the previous completed run of the same command, when one was found. */
  previousMetrics?: Record<string, number>;
  /** The run those metrics came from, for saying which run is being compared against. */
  previousRun?: Migration;
  /** True once the search for a previous run has finished. */
  comparisonReady: boolean;
};

/** A metric's change, with a verdict only where the direction is known. */
const Delta: React.FC<{ comparison: MetricComparison }> = ({ comparison }) => {
  if (comparison.delta === undefined) {
    return (
      <Typography variant="caption" color="text.secondary">
        —
      </Typography>
    );
  }

  const colour =
    comparison.verdict === 'better'
      ? 'success.main'
      : comparison.verdict === 'worse'
        ? 'error.main'
        : 'text.secondary';
  const sign = comparison.delta > 0 ? '+' : '';
  const pct =
    comparison.deltaPct === undefined ? '' : ` (${sign}${(comparison.deltaPct * 100).toFixed(1)}%)`;

  return (
    <Tooltip
      title={
        comparison.verdict === 'unknown'
          ? 'This metric has no known better direction, so the change is shown without a verdict.'
          : `Previous: ${formatMetricValue(comparison.previous ?? 0)}`
      }
    >
      <Typography variant="caption" color={colour} fontWeight={600}>
        {sign}
        {formatMetricValue(comparison.delta)}
        {pct}
      </Typography>
    </Tooltip>
  );
};

/** Where the data this run trained on came from. */
const Provenance: React.FC<{ provenance: NonNullable<RunMetadata['provenance']> }> = ({
  provenance,
}) => (
  <Box>
    <Typography variant="subtitle2" gutterBottom>
      Dataset used
    </Typography>
    <Typography variant="body2" component="div">
      {provenance.dataset_feed && (
        <>
          Feed <strong>{provenance.dataset_feed}</strong> ·{' '}
        </>
      )}
      <Tooltip title={provenance.dataset_sha256 ?? ''}>
        <span>sha256 {shortDigest(provenance.dataset_sha256)}</span>
      </Tooltip>
      {provenance.dataset_match_files != null && (
        <> · {formatCount(provenance.dataset_match_files)} match files</>
      )}
      {provenance.dataset_extracted_at && (
        <> · extracted {formatWhen(provenance.dataset_extracted_at)}</>
      )}
    </Typography>
    {provenance.dataset_source_url && (
      <Typography variant="caption" color="text.secondary" sx={{ wordBreak: 'break-all' }}>
        {provenance.dataset_source_url}
      </Typography>
    )}
  </Box>
);

/**
 * What a finished run did: which formats trained, on what, with which results, and
 * from which dataset.
 *
 * The comparison against the previous run is the reason this is worth building rather
 * than dumping JSON — a metric only means something next to the last one.
 */
const RunSummaryPanel: React.FC<Props> = ({
  metadata,
  previousMetrics,
  previousRun,
  comparisonReady,
}) => {
  const glossary = useMetricGlossary();
  const summary = metadata.summary;
  const formats = summary?.formats ?? [];
  const dropped = Object.entries(summary?.dropped_columns ?? {});

  const currentMetrics: Record<string, number> = {};
  for (const fmt of formats) {
    for (const [name, value] of Object.entries(fmt.metrics ?? {})) {
      currentMetrics[`${fmt.format}.${name}`] = value;
    }
  }
  const comparisons = compareMetrics(currentMetrics, previousMetrics, glossary);

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
      {summary && (
        <Typography variant="body2" color="text.secondary">
          {summary.formats_completed ?? 0} of {summary.formats_total ?? 0} formats ·{' '}
          {summary.saved ?? 0} model(s) saved
          {summary.finished_at && <> · finished {formatWhen(summary.finished_at)}</>}
        </Typography>
      )}

      {metadata.provenance ? (
        <Provenance provenance={metadata.provenance} />
      ) : (
        // Not a failure to report — a directory populated before the registry existed,
        // or by hand, genuinely has no provenance. Saying so beats an empty space.
        <Typography variant="body2" color="text.secondary">
          Dataset used: <em>unknown</em> — this run predates the dataset registry, or the data
          directory was populated directly.
        </Typography>
      )}

      {formats.length > 0 && (
        <Box>
          <Typography variant="subtitle2" gutterBottom>
            Per format
          </Typography>
          <Box sx={{ overflowX: 'auto' }}>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>Format</TableCell>
                  <TableCell align="right">Rows</TableCell>
                  <TableCell align="right">Features</TableCell>
                  <TableCell>Artifacts</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {formats.map((fmt) => (
                  <TableRow key={fmt.format}>
                    <TableCell>{fmt.format}</TableCell>
                    <TableCell align="right">{formatCount(fmt.rows)}</TableCell>
                    <TableCell align="right">{fmt.features ?? '—'}</TableCell>
                    <TableCell>
                      {(fmt.artifacts ?? [])
                        .map((a) => `${a.path} (${formatBytes(a.bytes)})`)
                        .join(', ') || '—'}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Box>
        </Box>
      )}

      {comparisons.length > 0 && (
        <Box>
          <Typography variant="subtitle2" gutterBottom>
            Metrics
            {previousRun ? (
              <Typography variant="caption" color="text.secondary" component="span" sx={{ ml: 1 }}>
                compared with run {previousRun.id} ({formatWhen(previousRun.started_at)})
              </Typography>
            ) : (
              comparisonReady && (
                <Typography
                  variant="caption"
                  color="text.secondary"
                  component="span"
                  sx={{ ml: 1 }}
                >
                  no earlier run of this step to compare against
                </Typography>
              )
            )}
          </Typography>
          <Box sx={{ overflowX: 'auto' }}>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>Metric</TableCell>
                  <TableCell align="right">Value</TableCell>
                  <TableCell align="right">Change</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {comparisons.map((c) => (
                  <TableRow key={c.name}>
                    <TableCell>
                      {/* `T20.objective_auc`: the format prefixes the key the glossary knows. */}
                      <MetricLabel
                        metricKey={c.name.split('.').pop() ?? c.name}
                        label={c.name}
                        value={formatMetricValue(c.current)}
                      />
                    </TableCell>
                    <TableCell align="right">{formatMetricValue(c.current)}</TableCell>
                    <TableCell align="right">
                      <Delta comparison={c} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Box>
        </Box>
      )}

      {/* The plan singles this out: a silently constant feature was being dropped with
          nobody told. On the run record it stays answerable after the fact. */}
      {dropped.length > 0 && (
        <Box>
          <Typography variant="subtitle2" gutterBottom>
            Low-variance columns dropped
          </Typography>
          {dropped.map(([fmt, names]) => (
            <Typography key={fmt} variant="body2" color="warning.main" component="div">
              <strong>{fmt}</strong>: {names.join(', ')}
            </Typography>
          ))}
          <Typography variant="caption" color="text.secondary">
            These were effectively constant in this dataset and were removed before fitting. A
            feature you expected to matter appearing here is worth chasing.
          </Typography>
        </Box>
      )}

      {formats.length === 0 && (
        <Chip size="small" label="This run recorded no per-format detail" variant="outlined" />
      )}
    </Box>
  );
};

export default RunSummaryPanel;
