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
  return isNaN(d.getTime()) ? iso : d.toLocaleString();
}

function ModelRow({ model }: { model: MLModelStat }) {
  const [open, setOpen] = useState(false);
  const params = model.tuned_parameters;
  const metrics = model.metrics;
  const hasDetails = (params && Object.keys(params).length > 0) || (metrics && Object.keys(metrics).length > 0);

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
        <TableCell>{model.accuracy_display ?? (model.best_cv_score != null ? String(model.best_cv_score) : '—')}</TableCell>
        <TableCell>{formatBytes(model.size_bytes)}</TableCell>
        <TableCell sx={{ fontSize: '0.85rem' }}>{formatModified(model.modified)}</TableCell>
        <TableCell>{model.tuned ? 'Yes' : 'No'}</TableCell>
      </TableRow>
      {hasDetails && (
        <TableRow>
          <TableCell style={{ paddingBottom: 0, paddingTop: 0 }} colSpan={8}>
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
                      {Object.entries(metrics).map(([k, v]) => (
                        <Chip
                          key={k}
                          label={`${k}=${String(v)}`}
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
          subtitle="Trained models with format, tuned parameters, algorithm, accuracy, and artifact size."
        >
          {data.models.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No model artifacts found. Train models via Ops Status → Pipeline (e.g. train-batting, train-bowling).
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
                    <TableCell>Size</TableCell>
                    <TableCell>Modified</TableCell>
                    <TableCell>Tuned</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {data.models.map((model, idx) => (
                    <ModelRow key={`${model.model_name}-${model.match_format}-${idx}`} model={model} />
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
