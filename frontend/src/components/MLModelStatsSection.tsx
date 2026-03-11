import React from 'react';
import type { ModelStatsResponse } from '../types';
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
import SectionCard from './common/SectionCard';
import { MLModelRow } from './MLModelRow';
import MLPredictionGraph from './MLPredictionGraph';
import OpsFormatHierarchy from './OpsFormatHierarchy';

/** Presentational section for ML model stats: table, refresh button, loading/error states. */
export interface MLModelStatsSectionProps {
  data: ModelStatsResponse | null;
  error: string | null;
  loading: boolean;
  onRefresh: () => void;
}

export function MLModelStatsSection({
  data,
  error,
  loading,
  onRefresh,
}: MLModelStatsSectionProps): JSX.Element {
  return (
    <Stack spacing={2}>
      <Stack direction="row" spacing={1} alignItems="center">
        <Button variant="contained" onClick={onRefresh} disabled={loading}>
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
        <>
          <SectionCard
            title="ML model stats"
            subtitle="Trained models with format, tuned parameters, algorithm, accuracy, size. Metrics include per-target MAE, baseline improvement, overfitting gap. MLQA Audit shows overfitting, stability, bias, and deployment readiness. Expand a row for tuning insights and full details."
          >
            {data.models.length === 0 ? (
              <Typography variant="body2" color="text.secondary">
                No model artifacts found. Train models via Ops Status → Pipeline (e.g.
                train-batting, train-bowling).
              </Typography>
            ) : (
              <TableContainer component={Paper} variant="outlined">
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
                      <MLModelRow key={`${model.model_name}-${model.match_format}`} model={model} />
                    ))}
                  </TableBody>
                </Table>
              </TableContainer>
            )}
          </SectionCard>

          <SectionCard
            title="Prediction model flow"
            subtitle="Features at cutoff → per-player models (batting, bowling, fielding) → team aggregates + extras → win model (winner and team scores reconciled to win probability) → team selection and simulation."
          >
            <MLPredictionGraph />
          </SectionCard>

          <SectionCard
            title="Match Type Hierarchy"
            subtitle="Structural hierarchy of match types (bucket → leaf) used by the pipeline for format-specific training and evaluation."
          >
            <OpsFormatHierarchy hierarchy={data.hierarchy} />
          </SectionCard>
        </>
      )}
    </Stack>
  );
}
