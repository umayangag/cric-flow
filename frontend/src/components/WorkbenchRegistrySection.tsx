import React from 'react';
import {
  Box,
  Button,
  Stack,
  Typography,
  Alert,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
} from '@mui/material';
import UploadFileIcon from '@mui/icons-material/UploadFile';
import SectionCard from './common/SectionCard';
import type { WalkForwardRegistry, WalkForwardWindowEntry } from '../types';

function formatDate(s: string): string {
  if (!s) return '—';
  try {
    const d = new Date(s);
    return Number.isNaN(d.getTime()) ? s : d.toISOString().slice(0, 10);
  } catch {
    return s;
  }
}

export interface WorkbenchRegistrySectionProps {
  registryFile: File | null;
  registryError: string | null;
  registry: WalkForwardRegistry | null;
  onFileChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
}

const WorkbenchRegistrySection: React.FC<WorkbenchRegistrySectionProps> = ({
  registryFile,
  registryError,
  registry,
  onFileChange,
}) => {
  const regMetricKeys =
    registry && registry.windows.length > 0
      ? Array.from(new Set(registry.windows.flatMap((w) => Object.keys(w.metrics || {})))).sort()
      : [];

  return (
    <SectionCard
      title="Walk-forward registry"
      subtitle="Upload a walk-forward registry JSON to view how well the model generalizes over time."
    >
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
        <strong>What it is:</strong> Walk-forward evaluation tests whether your model stays accurate
        as time moves forward. For each &quot;window&quot;, the pipeline trains on data only
        <em> before </em> a cutoff date, then predicts the next X matches (holdout), and compares
        predictions to actual results. The registry file records each window (model, format, cutoff,
        metrics like MAE). This helps you spot if the model degrades on newer matches.
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
        <strong>Why use it:</strong> A single backtest on a date range can hide that the model
        performs worse on recent data. Walk-forward simulates real use: train on the past, predict
        the future, then advance time and repeat. Upload the registry here to inspect metrics per
        window (e.g. n_train, n_holdout, runs_mae) without re-running the pipeline.
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
        <strong>How to get a registry:</strong> Run from the repo:{' '}
        <Box component="code" sx={{ fontSize: '0.85em', bgcolor: 'action.hover', px: 0.5 }}>
          make walk-forward INITIAL_CUTOFF=2024-01-01 WINDOW_X=50 WALK_FORMAT=T20
        </Box>{' '}
        (adjust dates and format as needed). The pipeline writes{' '}
        <code>walk_forward_registry.json</code> to the ML service output directory. Upload that file
        below.
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        <strong>Expected file shape:</strong> JSON with <code>run_id</code> (string) and{' '}
        <code>windows</code> (array). Each window has <code>model_type</code>, <code>format</code>,{' '}
        <code>cutoff_trained_before</code>, <code>window_x</code>, <code>metrics</code> (e.g.{' '}
        <code>runs_mae</code>), and optionally <code>n_training_samples</code>,{' '}
        <code>n_holdout_samples</code>. Example:
      </Typography>
      <Box
        component="pre"
        sx={{
          fontSize: 11,
          p: 1.5,
          bgcolor: 'grey.100',
          borderRadius: 1,
          overflow: 'auto',
          border: '1px solid',
          borderColor: 'divider',
          mb: 2,
        }}
      >
        {`{
  "run_id": "walk-2024-01-15",
  "windows": [
    {
      "model_type": "batting",
      "format": "T20",
      "cutoff_trained_before": "2024-01-01T00:00:00Z",
      "window_x": 50,
      "n_training_samples": 1200,
      "n_holdout_samples": 50,
      "metrics": { "runs_mae": 12.4, "player_runs_mae": 8.2 }
    }
  ],
  "config": { "initial_cutoff": "2024-01-01", "window_x": 50, "format": "T20" }
}`}
      </Box>
      <Stack direction="row" alignItems="center" spacing={2}>
        <Button variant="outlined" component="label" startIcon={<UploadFileIcon />}>
          Choose JSON file
          <input type="file" accept=".json,application/json" hidden onChange={onFileChange} />
        </Button>
        {registryFile && (
          <Typography variant="body2" color="text.secondary">
            {registryFile.name}
          </Typography>
        )}
      </Stack>
      {registryError && (
        <Alert severity="error" sx={{ mt: 2 }}>
          {registryError}
        </Alert>
      )}
      {registry && registry.windows.length > 0 && (
        <TableContainer component={Paper} variant="outlined" sx={{ maxHeight: 320, mt: 2 }}>
          <Table size="small" stickyHeader>
            <TableHead>
              <TableRow>
                <TableCell>#</TableCell>
                <TableCell>Model</TableCell>
                <TableCell>Format</TableCell>
                <TableCell>Cutoff (trained before)</TableCell>
                <TableCell>Window X</TableCell>
                <TableCell align="right">n_train</TableCell>
                <TableCell align="right">n_holdout</TableCell>
                {regMetricKeys.map((k) => (
                  <TableCell key={k} align="right">
                    {k}
                  </TableCell>
                ))}
              </TableRow>
            </TableHead>
            <TableBody>
              {registry.windows.map((w: WalkForwardWindowEntry, idx: number) => (
                <TableRow key={idx}>
                  <TableCell>{w.window_index ?? idx + 1}</TableCell>
                  <TableCell>{w.model_type}</TableCell>
                  <TableCell>{w.format}</TableCell>
                  <TableCell>{formatDate(w.cutoff_trained_before)}</TableCell>
                  <TableCell>{w.window_x}</TableCell>
                  <TableCell align="right">{w.n_training_samples ?? '—'}</TableCell>
                  <TableCell align="right">{w.n_holdout_samples ?? '—'}</TableCell>
                  {regMetricKeys.map((k) => (
                    <TableCell key={k} align="right">
                      {w.metrics?.[k] != null
                        ? typeof w.metrics[k] === 'number'
                          ? (w.metrics[k] as number).toFixed(3)
                          : String(w.metrics[k])
                        : '—'}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
      {registry && registry.windows.length === 0 && (
        <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
          No windows in this registry.
        </Typography>
      )}
    </SectionCard>
  );
};

export default WorkbenchRegistrySection;
