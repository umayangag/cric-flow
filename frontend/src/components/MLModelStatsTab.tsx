import React, { useCallback, useEffect, useState } from 'react';
import { api } from '../api';
import type { MLModelStat, ModelStatsResponse } from '../types';
import Stack from '@mui/material/Stack';
import Button from '@mui/material/Button';
import Typography from '@mui/material/Typography';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Paper from '@mui/material/Paper';
import Chip from '@mui/material/Chip';
import Box from '@mui/material/Box';
import Collapse from '@mui/material/Collapse';
import IconButton from '@mui/material/IconButton';
import KeyboardArrowDown from '@mui/icons-material/KeyboardArrowDown';
import KeyboardArrowUp from '@mui/icons-material/KeyboardArrowUp';
import SectionCard from './common/SectionCard';

function formatBytes(n: number | undefined): string {
  if (n == null || !isFinite(n) || n <= 0) return '—';
  const units = ['B', 'KB', 'MB', 'GB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v < 10 && i > 0 ? 1 : 0)} ${units[i]}`;
}

function formatModified(iso: string | undefined): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (isNaN(d.getTime())) {
    return iso;
  }
  const pad = (num: number) => num.toString().padStart(2, '0');
  const date = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
  return `${date} ${time}`;
}

function formatDuration(seconds: number | undefined): string {
  if (seconds == null || !isFinite(seconds) || seconds < 0) return '—';
  if (seconds < 60) return `${Math.round(seconds)}s`;
  const m = Math.floor(seconds / 60);
  const s = Math.round(seconds % 60);
  return s > 0 ? `${m}m ${s}s` : `${m}m`;
}

function getAccuracyDisplay(model: MLModelStat): string {
  if (model.accuracy_display) return model.accuracy_display;
  if (model.best_cv_score != null && isFinite(model.best_cv_score)) {
    const scoring = (model.scoring || '').toLowerCase();
    if (scoring.includes('neg_mean_absolute_error') || scoring.includes('neg_mae')) {
      // neg_MAE is not a percentage; show as MAE
      const mae = Math.abs(model.best_cv_score).toFixed(2);
      return model.scoring ? `MAE=${mae} (${model.scoring})` : `MAE=${mae}`;
    }
    const pct = (model.best_cv_score * 100).toFixed(1);
    return model.scoring ? `${pct}% (${model.scoring})` : `${pct}%`;
  }
  return '—';
}

function statusColor(status: 'PASS' | 'FAIL' | 'WARNING'): 'success' | 'error' | 'warning' {
  if (status === 'PASS') return 'success';
  if (status === 'FAIL') return 'error';
  return 'warning';
}

/** Flatten metrics for display; expand nested objects so chips show key=value instead of [object Object]. */
function flattenMetricsForDisplay(
  metrics: Record<string, unknown>,
): Array<[string, string | number | boolean]> {
  const out: Array<[string, string | number | boolean]> = [];
  for (const [k, v] of Object.entries(metrics)) {
    if (v == null) continue;
    // The backend is expected to flatten nested objects.
    // Any remaining objects are skipped to avoid '[object Object]' in chips.
    if (typeof v === 'object' && !Array.isArray(v)) {
      continue;
    }
    if (Array.isArray(v)) {
      out.push([k, v.map(String).join(', ')]);
      continue;
    }
    out.push([k, v as string | number | boolean]);
  }
  return out;
}

/** Highlight key tuning/eval metrics for quick assessment. */
function TuningInsights({
  metrics,
  mlqa,
}: {
  metrics: Record<string, unknown>;
  mlqa?: { checks?: { stability?: { cv_std?: number; cv_fold_scores?: number[] } } };
}): JSX.Element | null {
  const insightDefinitions: Array<{
    label: string;
    value: unknown;
    condition: (v: unknown) => boolean;
    format: (v: unknown) => string;
    hint?: string;
  }> = [
    {
      label: 'Baseline improvement',
      value: metrics.baseline_improvement_pct as number,
      condition: (v) => typeof v === 'number',
      format: (v) => `${v as number}%`,
      hint: 'vs naive (predict mean); higher = model adds more value',
    },
    {
      label: 'MAE % of mean',
      value: metrics.mae_pct_of_mean as number,
      condition: (v) => typeof v === 'number',
      format: (v) => `${v as number}%`,
      hint: 'relative error; lower is better',
    },
    {
      label: 'Overfitting gap',
      value: metrics.overfitting_gap as number,
      condition: (v) => typeof v === 'number',
      format: (v) => (v as number).toFixed(4),
      hint: 'train−val score diff; high = overfitting',
    },
    {
      label: 'Val still improving',
      value: metrics.val_still_improving as boolean,
      condition: (v) => typeof v === 'boolean',
      format: (v) => ((v as boolean) ? 'Yes' : 'No'),
      hint: 'more data might help if Yes',
    },
    {
      label: 'CV fold σ',
      value: mlqa?.checks?.stability?.cv_std as number,
      condition: (v) => typeof v === 'number',
      format: (v) => (v as number).toFixed(4),
      hint: 'stability; >0.05 = unstable',
    },
    {
      label: 'CV fold scores',
      value: mlqa?.checks?.stability?.cv_fold_scores as number[],
      condition: (v) => Array.isArray(v) && v.length > 0,
      format: (v) => (v as number[]).map(String).join(', '),
      hint: 'per-fold scores (neg_MAE)',
    },
  ];

  const items = insightDefinitions
    .filter(({ value, condition }) => value != null && condition(value))
    .map(({ label, value, format, hint }) => ({
      label,
      value: format(value),
      hint,
    }));

  if (items.length === 0) return null;
  return (
    <Box sx={{ mt: 1.5, p: 1, bgcolor: 'action.hover', borderRadius: 1 }}>
      <Typography
        variant="caption"
        color="text.secondary"
        sx={{ fontWeight: 600, display: 'block', mb: 0.5 }}
      >
        Tuning insights
      </Typography>
      <Stack spacing={0.5}>
        {items.map(({ label, value, hint }) => (
          <Typography
            key={label}
            variant="caption"
            component="div"
            sx={{ fontFamily: 'monospace' }}
          >
            <Box component="span" sx={{ fontWeight: 600, mr: 0.5 }}>
              {label}:
            </Box>
            {value}
            {hint && (
              <Typography
                component="span"
                variant="caption"
                color="text.secondary"
                sx={{ ml: 0.5, fontStyle: 'italic' }}
              >
                ({hint})
              </Typography>
            )}
          </Typography>
        ))}
      </Stack>
    </Box>
  );
}

function ModelRow({ model }: { model: MLModelStat }) {
  const [open, setOpen] = useState(false);
  const params = model.tuned_parameters;
  const metrics = model.metrics;
  const featureImportance = model.feature_importance;
  const mlqa = model.mlqa_audit;
  const hasDetails =
    (params && Object.keys(params).length > 0) ||
    (metrics && Object.keys(metrics).length > 0) ||
    (featureImportance && Object.keys(featureImportance).length > 0) ||
    (mlqa && (mlqa.key_findings?.length > 0 || mlqa.final_verdict));

  return (
    <>
      <TableRow sx={{ '& > *': { borderBottom: 'unset' } }}>
        <TableCell>
          {hasDetails && (
            <IconButton
              aria-label={open ? 'hide details' : 'show details'}
              size="small"
              onClick={() => setOpen(!open)}
            >
              {open ? <KeyboardArrowUp /> : <KeyboardArrowDown />}
            </IconButton>
          )}
        </TableCell>
        <TableCell component="th" scope="row">
          {model.model_name}
        </TableCell>
        <TableCell>
          <Chip label={model.match_format} size="small" variant="outlined" />
        </TableCell>
        <TableCell>{model.algorithm ?? '—'}</TableCell>
        <TableCell>{getAccuracyDisplay(model)}</TableCell>
        <TableCell>
          {model.mlqa_audit ? (
            <Chip
              label={model.mlqa_audit.audit_status}
              size="small"
              color={statusColor(model.mlqa_audit.audit_status)}
            />
          ) : (
            '—'
          )}
        </TableCell>
        <TableCell>{formatBytes(model.size_bytes)}</TableCell>
        <TableCell sx={{ fontSize: '0.85rem' }}>{formatModified(model.modified)}</TableCell>
        <TableCell>{model.tuned ? 'Yes' : 'No'}</TableCell>
        <TableCell sx={{ fontSize: '0.85rem' }}>{formatModified(model.trained_at)}</TableCell>
        <TableCell>{formatDuration(model.duration_seconds)}</TableCell>
      </TableRow>
      {hasDetails && (
        <TableRow>
          <TableCell sx={{ py: 0 }} colSpan={11}>
            <Collapse in={open} timeout="auto" unmountOnExit>
              <Box sx={{ py: 2, px: 1 }}>
                {params && Object.keys(params).length > 0 && (
                  <Box sx={{ mb: 1 }}>
                    <Typography variant="subtitle2" gutterBottom>
                      Tuned parameters
                    </Typography>
                    <Stack direction="row" flexWrap="wrap" spacing={0.5}>
                      {Object.entries(params).map(([k, v]) => (
                        <Chip
                          key={k}
                          label={`${k}=${String(v)}`}
                          size="small"
                          variant="filled"
                          sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}
                        />
                      ))}
                    </Stack>
                  </Box>
                )}
                {metrics && Object.keys(metrics).length > 0 && (
                  <Box>
                    <Typography variant="subtitle2" gutterBottom>
                      Metrics
                    </Typography>
                    <Stack direction="row" flexWrap="wrap" spacing={0.5}>
                      {flattenMetricsForDisplay(metrics as Record<string, unknown>).map(
                        ([k, v]) => (
                          <Chip
                            key={k}
                            label={`${k}=${String(v)}`}
                            size="small"
                            variant="outlined"
                            sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}
                          />
                        ),
                      )}
                    </Stack>
                    <TuningInsights metrics={metrics} mlqa={mlqa} />
                  </Box>
                )}
                {featureImportance && Object.keys(featureImportance).length > 0 && (
                  <Box sx={{ mt: 1 }}>
                    <Typography variant="subtitle2" gutterBottom>
                      Feature importance (top)
                    </Typography>
                    <Stack direction="row" flexWrap="wrap" spacing={0.5}>
                      {Object.entries(featureImportance)
                        .sort(([, a], [, b]) => b - a)
                        .slice(0, 15)
                        .map(([name, imp]) => (
                          <Chip
                            key={name}
                            label={`${name}: ${(imp * 100).toFixed(1)}%`}
                            size="small"
                            variant="outlined"
                            sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}
                          />
                        ))}
                    </Stack>
                  </Box>
                )}
                {model.n_samples != null && (
                  <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                    n_samples: {model.n_samples}
                    {model.n_features != null && ` · n_features: ${model.n_features}`}
                    {model.cv_splits != null && ` · cv_splits: ${model.cv_splits}`}
                    {model.validation_method && ` · validation: ${model.validation_method}`}
                  </Typography>
                )}
                {mlqa && (
                  <Box sx={{ mt: 2 }}>
                    <Typography variant="subtitle2" gutterBottom>
                      MLQA Audit
                    </Typography>
                    <Stack spacing={0.5}>
                      <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
                        <Chip
                          label={mlqa.audit_status}
                          size="small"
                          color={statusColor(mlqa.audit_status)}
                          sx={{ fontWeight: 600 }}
                        />
                        <Chip
                          label={mlqa.final_verdict}
                          size="small"
                          variant="outlined"
                          sx={{ fontFamily: 'monospace' }}
                        />
                      </Stack>
                      {mlqa.key_findings && mlqa.key_findings.length > 0 && (
                        <Box component="ul" sx={{ m: 0, pl: 2, fontSize: '0.85rem' }}>
                          {mlqa.key_findings.map((f, i) => (
                            <li key={i}>{f}</li>
                          ))}
                        </Box>
                      )}
                      {mlqa.bias_report && (
                        <Typography
                          variant="body2"
                          color="text.secondary"
                          sx={{ fontStyle: 'italic' }}
                        >
                          {mlqa.bias_report}
                        </Typography>
                      )}
                      {mlqa.checks && (
                        <Stack direction="row" spacing={1} flexWrap="wrap" sx={{ mt: 0.5 }}>
                          {mlqa.checks.overfitting && (
                            <Chip
                              label={`Δ=${mlqa.checks.overfitting.delta} ${mlqa.checks.overfitting.flagged ? '⚠' : '✓'}`}
                              size="small"
                              variant="outlined"
                            />
                          )}
                          {mlqa.checks.stability && (
                            <Chip
                              label={`σ=${mlqa.checks.stability.cv_std} ${mlqa.checks.stability.flagged ? '⚠' : '✓'}`}
                              size="small"
                              variant="outlined"
                            />
                          )}
                        </Stack>
                      )}
                    </Stack>
                  </Box>
                )}
              </Box>
            </Collapse>
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

const MLModelStatsTab: React.FC = () => {
  const [data, setData] = useState<ModelStatsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const fetchStats = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await api.getModelStats();
      setData(res);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to fetch model stats');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchStats();
  }, [fetchStats]);

  return (
    <Stack spacing={2}>
      <Stack direction="row" spacing={1} alignItems="center">
        <Button variant="contained" onClick={fetchStats} disabled={loading}>
          {loading ? 'Refreshing…' : 'Refresh'}
        </Button>
        {data?.models_dir && (
          <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
            {data.models_dir}
          </Typography>
        )}
      </Stack>

      {error && (
        <Typography color="error" role="alert">
          {error}
        </Typography>
      )}

      {!data && !error && (
        <Typography variant="body2" sx={{ opacity: 0.8 }}>
          <em>Loading ML model stats…</em>
        </Typography>
      )}

      {data && (
        <SectionCard
          title="ML model stats"
          subtitle="Trained models with format, tuned parameters, algorithm, accuracy, size. Metrics include per-target MAE, baseline improvement, overfitting gap. MLQA Audit shows overfitting, stability, bias, and deployment readiness. Expand a row for tuning insights and full details."
        >
          {data.models.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No model artifacts found. Train models via Ops Status → Pipeline (e.g. train-batting,
              train-bowling).
            </Typography>
          ) : (
            <TableContainer component={Paper} variant="outlined" sx={{ maxHeight: 560 }}>
              <Table stickyHeader size="small">
                <TableHead>
                  <TableRow>
                    <TableCell />
                    <TableCell>Model</TableCell>
                    <TableCell>Match format</TableCell>
                    <TableCell>Algorithm</TableCell>
                    <TableCell>Accuracy / score</TableCell>
                    <TableCell>Audit</TableCell>
                    <TableCell>Size</TableCell>
                    <TableCell>Modified</TableCell>
                    <TableCell>Tuned</TableCell>
                    <TableCell>Trained at</TableCell>
                    <TableCell>Duration</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {data.models.map((model) => (
                    <ModelRow key={`${model.model_name}-${model.match_format}`} model={model} />
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </SectionCard>
      )}
    </Stack>
  );
};

export default MLModelStatsTab;
