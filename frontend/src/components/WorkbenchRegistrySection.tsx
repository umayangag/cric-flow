import React from 'react';
import {
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
      {/*
        What survives the W2-2 audit: why walk-forward exists at all, which is a
        judgement about evaluation and is not derivable from anything on screen.
        What did not: a Makefile invocation with env vars (the Commands & docs card
        in this same tab already carries it, and two copies drift apart) and a
        hand-written description of the file's JSON keys with a worked example
        (the parser knows the shape — so the parser says so, when a file is wrong).
      */}
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        A single backtest over a date range can hide that a model does worse on recent matches.
        Walk-forward evaluation trains on data before a cutoff, predicts the next X matches, then
        advances the cutoff and repeats — so each window is scored on matches its model never saw.
        Upload a registry to read those per-window metrics without re-running the pipeline. The
        command that produces one is in <strong>Commands &amp; docs</strong> below.
      </Typography>
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
