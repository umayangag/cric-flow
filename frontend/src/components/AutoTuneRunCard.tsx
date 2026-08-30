import React from 'react';
import { Box } from '@mui/material';
import type { AutoTuneRunDetailsEntry } from '../types';

export interface AutoTuneRunCardProps {
  run: AutoTuneRunDetailsEntry;
}

const AutoTuneRunCard: React.FC<AutoTuneRunCardProps> = ({ run }) => {
  const params = (run.params ?? {}) as Record<string, unknown>;
  const metrics = (run.metrics ?? {}) as Record<string, unknown>;
  const algorithms = Array.isArray(params.algorithms) ? (params.algorithms as string[]) : [];
  const algorithm =
    (params.algorithm as string | undefined) || (algorithms.length > 0 ? algorithms[0] : undefined);
  const validationMethod =
    (params.validation_method as string | undefined) ||
    (metrics.validation_method as string | undefined);
  const bestCvScore = metrics.best_cv_score as number | undefined;
  const scoring = metrics.scoring as string | undefined;
  const mlqa = metrics.mlqa_audit as
    | {
        audit_status?: string;
        final_verdict?: string;
        key_findings?: string[];
      }
    | undefined;

  return (
    <Box
      sx={{
        p: 1.5,
        borderRadius: 1,
        border: '1px solid',
        borderColor: 'divider',
        bgcolor: 'grey.50',
        fontSize: 13,
      }}
    >
      <strong>
        {run.model} — {run.format || 'no format'} (saved at{' '}
        {new Date(run.created_at).toLocaleString()})
      </strong>
      <Box component="div" sx={{ mt: 1 }}>
        <div>
          <strong>Selected algorithm:</strong> {algorithm ?? 'Unknown'}
        </div>
        {validationMethod && (
          <div>
            <strong>Validation:</strong> {validationMethod}
          </div>
        )}
        {scoring && (
          <div>
            <strong>Scoring metric:</strong> {scoring}
          </div>
        )}
        {typeof bestCvScore === 'number' && (
          <div>
            <strong>Best CV score:</strong> {bestCvScore.toFixed(4)}
          </div>
        )}
      </Box>

      {mlqa && (
        <Box component="div" sx={{ mt: 1 }}>
          <strong>MLQA audit:</strong>
          <div>Status: {mlqa.audit_status ?? 'N/A'}</div>
          {mlqa.final_verdict && <div>Verdict: {mlqa.final_verdict}</div>}
          {Array.isArray(mlqa.key_findings) && mlqa.key_findings.length > 0 && (
            <ul>
              {mlqa.key_findings.map((k, idx) => (
                <li key={`${k}-${idx}`}>{k}</li>
              ))}
            </ul>
          )}
        </Box>
      )}

      <Box
        component="pre"
        sx={{
          mt: 1.5,
          p: 1,
          bgcolor: 'grey.100',
          borderRadius: 1,
          overflow: 'auto',
          maxHeight: '30vh',
          fontSize: 11,
          fontFamily: 'ui-monospace, Menlo, monospace',
        }}
      >
        {JSON.stringify(
          {
            params,
            metrics,
          },
          null,
          2,
        )}
      </Box>
    </Box>
  );
};

export default AutoTuneRunCard;
