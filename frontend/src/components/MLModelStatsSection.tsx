import React, { useEffect, useMemo, useState } from 'react';
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
import TextField from '@mui/material/TextField';
import MenuItem from '@mui/material/MenuItem';
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
  const [modelFilter, setModelFilter] = useState<string>('all');
  const [formatFilter, setFormatFilter] = useState<string>('all');

  // Persist filters across page refreshes (best-effort; no-ops during SSR).
  useEffect(() => {
    if (typeof window === 'undefined') return;
    try {
      const raw = window.localStorage.getItem('mlModelStats.filters');
      if (!raw) return;
      const parsed = JSON.parse(raw) as { modelFilter?: string; formatFilter?: string } | null;
      if (parsed?.modelFilter) {
        setModelFilter(parsed.modelFilter);
      }
      if (parsed?.formatFilter) {
        setFormatFilter(parsed.formatFilter);
      }
    } catch {
      // Ignore malformed localStorage; fall back to defaults.
    }
  }, []);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    try {
      const payload = JSON.stringify({ modelFilter, formatFilter });
      window.localStorage.setItem('mlModelStats.filters', payload);
    } catch {
      // Ignore quota or access errors; filters will just not persist.
    }
  }, [modelFilter, formatFilter]);

  const { modelNames, matchFormats, filteredModels } = useMemo(() => {
    const modelNamesSet = new Set<string>();
    const matchFormatsSet = new Set<string>();

    const allModels = data?.models ?? [];

    for (const model of allModels) {
      if (model.model_name) {
        modelNamesSet.add(model.model_name);
      }
      if (model.match_format) {
        matchFormatsSet.add(model.match_format);
      }
    }

    const modelNames = Array.from(modelNamesSet).sort((a, b) => a.localeCompare(b));
    const matchFormats = Array.from(matchFormatsSet).sort((a, b) => a.localeCompare(b));

    const filteredModels = allModels.filter((model) => {
      const matchesModel = modelFilter === 'all' || model.model_name === modelFilter;
      const matchesFormat = formatFilter === 'all' || model.match_format === formatFilter;
      return matchesModel && matchesFormat;
    });

    return { modelNames, matchFormats, filteredModels };
  }, [data, modelFilter, formatFilter]);

  const hasAnyModels = (data?.models?.length ?? 0) > 0;
  const hasFilteredModels = filteredModels.length > 0;

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
            {hasAnyModels && (
              <Stack
                direction={{ xs: 'column', sm: 'row' }}
                spacing={2}
                alignItems={{ xs: 'stretch', sm: 'center' }}
                justifyContent="flex-start"
                sx={{ mb: 2 }}
              >
                <TextField
                  select
                  size="small"
                  label="Model"
                  value={modelFilter}
                  onChange={(event) => setModelFilter(event.target.value)}
                  sx={{ minWidth: 180 }}
                >
                  <MenuItem value="all">All models</MenuItem>
                  {modelNames.map((name) => (
                    <MenuItem key={name} value={name}>
                      {name}
                    </MenuItem>
                  ))}
                </TextField>

                <TextField
                  select
                  size="small"
                  label="Match format"
                  value={formatFilter}
                  onChange={(event) => setFormatFilter(event.target.value)}
                  sx={{ minWidth: 180 }}
                >
                  <MenuItem value="all">All formats</MenuItem>
                  {matchFormats.map((format) => (
                    <MenuItem key={format} value={format}>
                      {format}
                    </MenuItem>
                  ))}
                </TextField>
              </Stack>
            )}

            {!hasAnyModels ? (
              <Typography variant="body2" color="text.secondary">
                No model artifacts found. Train models via Ops Status → Pipeline (e.g.
                train-batting, train-bowling).
              </Typography>
            ) : !hasFilteredModels ? (
              <Typography variant="body2" color="text.secondary">
                No models match the selected filters.
              </Typography>
            ) : (
              <TableContainer
                component={Paper}
                variant="outlined"
                sx={{ maxHeight: 560, overflowY: 'auto' }}
              >
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
                    {filteredModels.map((model) => (
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
