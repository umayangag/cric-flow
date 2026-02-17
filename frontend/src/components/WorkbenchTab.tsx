import React, { useState, useEffect, useCallback } from 'react';
import {
  Box,
  Button,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Typography,
  Alert,
  CircularProgress,
  TextField,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  LinearProgress,
} from '@mui/material';
import UploadFileIcon from '@mui/icons-material/UploadFile';
import SectionCard from './common/SectionCard';
import { api } from '../api';
import type {
  AccuracyTrendResponse,
  AccuracyTrendFilters,
  AccuracyTrendItem,
  WalkForwardRegistry,
  WalkForwardWindowEntry,
} from '../types';

const DEFAULT_LIMIT = 100;
const MAX_LIMIT = 500;

const WorkbenchTab: React.FC = () => {
  const [format, setFormat] = useState<string>('');
  const [startDate, setStartDate] = useState<string>('');
  const [endDate, setEndDate] = useState<string>('');
  const [team1, setTeam1] = useState<string>('');
  const [team2, setTeam2] = useState<string>('');
  const [limit, setLimit] = useState<number>(DEFAULT_LIMIT);
  const [availableFormats, setAvailableFormats] = useState<string[]>([]);
  const [trendLoading, setTrendLoading] = useState(false);
  const [trendError, setTrendError] = useState<string | null>(null);
  const [trendData, setTrendData] = useState<AccuracyTrendResponse | null>(null);

  const [registryFile, setRegistryFile] = useState<File | null>(null);
  const [registryError, setRegistryError] = useState<string | null>(null);
  const [registry, setRegistry] = useState<WalkForwardRegistry | null>(null);

  useEffect(() => {
    let active = true;
    api
      .getFormats()
      .then((f) => {
        if (active) setAvailableFormats(f);
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, []);

  const loadAccuracyTrend = useCallback(async () => {
    setTrendError(null);
    setTrendLoading(true);
    try {
      const filters: AccuracyTrendFilters = {
        format: format || undefined,
        start_date: startDate || undefined,
        end_date: endDate || undefined,
        team1: team1 || undefined,
        team2: team2 || undefined,
        order: 'asc',
        limit: Math.min(Math.max(1, limit), MAX_LIMIT),
        cache: 'read',
      };
      const data = await api.accuracyTrend(filters);
      setTrendData(data);
    } catch (e) {
      setTrendError(e instanceof Error ? e.message : String(e));
      setTrendData(null);
    } finally {
      setTrendLoading(false);
    }
  }, [format, startDate, endDate, team1, team2, limit]);

  const handleRegistryFile = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    setRegistryError(null);
    setRegistry(null);
    setRegistryFile(file || null);
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      try {
        const text = reader.result as string;
        const parsed = JSON.parse(text) as WalkForwardRegistry;
        if (!parsed.windows || !Array.isArray(parsed.windows)) {
          setRegistryError('Invalid registry: missing "windows" array');
          return;
        }
        setRegistry(parsed);
      } catch (err) {
        setRegistryError(err instanceof Error ? err.message : 'Invalid JSON');
      }
    };
    reader.readAsText(file);
  };

  const formatDate = (s: string) => {
    if (!s) return '—';
    try {
      const d = new Date(s);
      return Number.isNaN(d.getTime()) ? s : d.toISOString().slice(0, 10);
    } catch {
      return s;
    }
  };

  const metricKeys = trendData?.results?.[0]
    ? Object.keys(trendData.results[0].metrics).sort()
    : [];

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
      <SectionCard
        title="Dataset & accuracy trend"
        subtitle="Filter by format and date range, then load backtest accuracy (MAE etc.) from the go-app API."
      >
        <Stack
          direction={{ xs: 'column', sm: 'row' }}
          spacing={2}
          flexWrap="wrap"
          alignItems="flex-start"
        >
          <FormControl size="small" sx={{ minWidth: 120 }}>
            <InputLabel>Format</InputLabel>
            <Select value={format} label="Format" onChange={(e) => setFormat(e.target.value)}>
              <MenuItem value="">All</MenuItem>
              {availableFormats.map((f) => (
                <MenuItem key={f} value={f}>
                  {f}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <TextField
            size="small"
            label="Start date"
            type="date"
            value={startDate}
            onChange={(e) => setStartDate(e.target.value)}
            InputLabelProps={{ shrink: true }}
            sx={{ width: 160 }}
          />
          <TextField
            size="small"
            label="End date"
            type="date"
            value={endDate}
            onChange={(e) => setEndDate(e.target.value)}
            InputLabelProps={{ shrink: true }}
            sx={{ width: 160 }}
          />
          <TextField
            size="small"
            label="Limit"
            type="number"
            value={limit}
            onChange={(e) => setLimit(Number(e.target.value) || DEFAULT_LIMIT)}
            inputProps={{ min: 1, max: MAX_LIMIT }}
            sx={{ width: 90 }}
          />
          <Button
            variant="contained"
            onClick={loadAccuracyTrend}
            disabled={trendLoading}
            startIcon={trendLoading ? <CircularProgress size={16} color="inherit" /> : null}
          >
            {trendLoading ? 'Loading…' : 'Load accuracy trend'}
          </Button>
        </Stack>
        {trendError && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {trendError}
          </Alert>
        )}
        {trendLoading && <LinearProgress sx={{ mt: 1 }} />}
        {trendData && !trendLoading && (
          <Box sx={{ mt: 2 }}>
            <Typography variant="body2" color="text.secondary" gutterBottom>
              Matches: {trendData.count}
              {Object.keys(trendData.summary || {}).length > 0 && (
                <>
                  {' '}
                  · Summary:{' '}
                  {Object.entries(trendData.summary)
                    .filter(([k]) => k !== 'n')
                    .map(([k, v]) => `${k}=${typeof v === 'number' ? v.toFixed(2) : v}`)
                    .join(', ')}
                </>
              )}
            </Typography>
            <TableContainer component={Paper} variant="outlined" sx={{ maxHeight: 360, mt: 1 }}>
              <Table size="small" stickyHeader>
                <TableHead>
                  <TableRow>
                    <TableCell>Match ID</TableCell>
                    <TableCell>Date</TableCell>
                    <TableCell>Format</TableCell>
                    <TableCell>Team1</TableCell>
                    <TableCell>Team2</TableCell>
                    {metricKeys.map((k) => (
                      <TableCell key={k} align="right">
                        {k}
                      </TableCell>
                    ))}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {(trendData.results || []).slice(0, 200).map((row: AccuracyTrendItem) => (
                    <TableRow key={row.match_id}>
                      <TableCell>{row.match_id}</TableCell>
                      <TableCell>{formatDate(row.match_date)}</TableCell>
                      <TableCell>{row.format}</TableCell>
                      <TableCell>{row.team1}</TableCell>
                      <TableCell>{row.team2}</TableCell>
                      {metricKeys.map((k) => (
                        <TableCell key={k} align="right">
                          {row.metrics[k] != null ? Number(row.metrics[k]).toFixed(2) : '—'}
                        </TableCell>
                      ))}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
            {(trendData.results?.length ?? 0) > 200 && (
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ mt: 0.5, display: 'block' }}
              >
                Showing first 200 of {trendData.results?.length} rows.
              </Typography>
            )}
          </Box>
        )}
      </SectionCard>

      <SectionCard
        title="Walk-forward registry"
        subtitle="Upload walk_forward_registry.json (from make walk-forward) to view MAE and variation per window."
      >
        <Stack direction="row" alignItems="center" spacing={2}>
          <Button variant="outlined" component="label" startIcon={<UploadFileIcon />}>
            Choose JSON file
            <input
              type="file"
              accept=".json,application/json"
              hidden
              onChange={handleRegistryFile}
            />
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
        {registry &&
          registry.windows.length > 0 &&
          (() => {
            const regMetricKeys = Array.from(
              new Set(registry.windows.flatMap((w) => Object.keys(w.metrics || {}))),
            ).sort();
            return (
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
            );
          })()}
        {registry && registry.windows.length === 0 && (
          <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
            No windows in this registry.
          </Typography>
        )}
      </SectionCard>

      <SectionCard
        title="Run & docs"
        subtitle="How to generate accuracy trend and walk-forward data."
      >
        <Typography variant="body2" paragraph>
          Accuracy trend is computed by the go-app backtest API. Ensure precompute and ML artifacts
          are in place; then use the filters above and click &quot;Load accuracy trend&quot;.
        </Typography>
        <Typography variant="body2" paragraph>
          Walk-forward: from repo root run{' '}
          <Box component="code" sx={{ bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}>
            make walk-forward INITIAL_CUTOFF=2020-01-01T00:00:00Z WINDOW_X=50 WALK_FORMAT=T20
          </Box>
          . The registry JSON is written to the ML service output dir (e.g.{' '}
          <code>walk_forward_registry.json</code>). Upload it above to view MAE and sample counts
          per window.
        </Typography>
        <Typography variant="body2">
          Auto-tune:{' '}
          <Box component="code" sx={{ bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}>
            make ml-auto-tune MODEL=batting FORMAT=T20
          </Box>
          . See <code>docs/ML_WALK_FORWARD.md</code> and <code>docs/ML_AUTO_TUNE.md</code> in the
          repo.
        </Typography>
      </SectionCard>
    </Box>
  );
};

export default WorkbenchTab;
