import React from 'react';
import type { MLModelStat } from '../types';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import Chip from '@mui/material/Chip';
import { Box } from '@mui/material';

function statusColor(status: 'PASS' | 'FAIL' | 'WARNING'): 'success' | 'error' | 'warning' {
  if (status === 'PASS') return 'success';
  if (status === 'FAIL') return 'error';
  return 'warning';
}

function flattenMetricsForDisplay(
  metrics: Record<string, unknown>,
): Array<[string, string | number | boolean]> {
  const out: Array<[string, string | number | boolean]> = [];
  for (const [k, v] of Object.entries(metrics)) {
    if (v == null) continue;
    if (typeof v === 'object' && !Array.isArray(v)) continue;
    if (Array.isArray(v)) {
      out.push([k, v.map(String).join(', ')]);
      continue;
    }
    out.push([k, v as string | number | boolean]);
  }
  return out;
}

function TuningInsights({
  metrics,
  mlqa,
}: {
  metrics: Record<string, unknown>;
  mlqa?: { checks?: { stability?: { cv_std?: number; cv_fold_scores?: number[] } } };
}): JSX.Element | null {
  const defs: Array<{
    label: string;
    value: unknown;
    condition: (v: unknown) => boolean;
    format: (v: unknown) => string;
    hint?: string;
  }> = [
    {
      label: 'Baseline improvement',
      value: metrics.baseline_improvement_pct,
      condition: (v) => typeof v === 'number',
      format: (v) => `${v as number}%`,
      hint: 'vs naive (predict mean); higher = model adds more value',
    },
    {
      label: 'MAE % of mean',
      value: metrics.mae_pct_of_mean,
      condition: (v) => typeof v === 'number',
      format: (v) => `${v as number}%`,
      hint: 'relative error; lower is better',
    },
    {
      label: 'Overfitting gap',
      value: metrics.overfitting_gap,
      condition: (v) => typeof v === 'number',
      format: (v) => (v as number).toFixed(4),
      hint: 'train−val score diff; high = overfitting',
    },
    {
      label: 'Val still improving',
      value: metrics.val_still_improving,
      condition: (v) => typeof v === 'boolean',
      format: (v) => ((v as boolean) ? 'Yes' : 'No'),
      hint: 'more data might help if Yes',
    },
    {
      label: 'CV fold σ',
      value: mlqa?.checks?.stability?.cv_std,
      condition: (v) => typeof v === 'number',
      format: (v) => (v as number).toFixed(4),
      hint: 'stability; >0.05 = unstable',
    },
    {
      label: 'CV fold scores',
      value: mlqa?.checks?.stability?.cv_fold_scores,
      condition: (v) => Array.isArray(v) && v.length > 0,
      format: (v) => (v as number[]).map(String).join(', '),
      hint: 'per-fold scores (neg_MAE)',
    },
  ];
  const items = defs
    .filter(({ value, condition }) => value != null && condition(value))
    .map(({ label, value, format, hint }) => ({ label, value: format(value), hint }));
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

export interface MLModelRowDetailsProps {
  model: MLModelStat;
}

export const MLModelRowDetails: React.FC<MLModelRowDetailsProps> = ({ model }) => {
  const params = model.tuned_parameters;
  const metrics = model.metrics;
  const featureImportance = model.feature_importance;
  const mlqa = model.mlqa_audit;

  return (
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
            {flattenMetricsForDisplay(metrics as Record<string, unknown>).map(([k, v]) => (
              <Chip
                key={k}
                label={`${k}=${String(v)}`}
                size="small"
                variant="outlined"
                sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}
              />
            ))}
          </Stack>
          <TuningInsights metrics={metrics} mlqa={mlqa} />
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
              <Typography variant="body2" color="text.secondary" sx={{ fontStyle: 'italic' }}>
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
            {featureImportance && Object.keys(featureImportance).length > 0 && (
              <Box sx={{ mt: 1 }}>
                <Typography variant="subtitle2" gutterBottom>
                  Feature importance (SHAP, top)
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
          </Stack>
        </Box>
      )}
    </Box>
  );
};

export default MLModelRowDetails;
