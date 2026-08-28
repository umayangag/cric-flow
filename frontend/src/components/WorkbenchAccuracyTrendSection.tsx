import React from 'react';
import ErrorNotice from './common/ErrorNotice';
import AccuracyTrendRuns from './AccuracyTrendRuns';
import type { Migration } from '../types';
import type { ApiError } from '../lib/apiError';
import {
  Box,
  Button,
  CircularProgress,
  FormControl,
  InputLabel,
  LinearProgress,
  MenuItem,
  Paper,
  Select,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import type { AccuracyTrendItem, AccuracyTrendResponse } from '../types';
import SectionCard from './common/SectionCard';

const formatMetricValue = (key: string, value: number | undefined): string => {
  if (value == null || Number.isNaN(value)) return '—';
  if (key.toLowerCase().includes('accuracy') || key.toLowerCase().endsWith('_acc')) {
    return `${(value * 100).toFixed(1)}%`;
  }
  return value.toFixed(2);
};

type Props = {
  format: string;
  availableFormats: string[];
  startDate: string;
  endDate: string;
  limit: number;
  maxLimit: number;
  trendLoading: boolean;
  trendError: ApiError | null;
  runs: Migration[];
  runsLoading: boolean;
  trendData: AccuracyTrendResponse | null;
  onChangeFormat: (value: string) => void;
  onChangeStartDate: (value: string) => void;
  onChangeEndDate: (value: string) => void;
  onChangeLimit: (value: number) => void;
  onLoad: () => void;
};

const WorkbenchAccuracyTrendSection: React.FC<Props> = ({
  format,
  availableFormats,
  startDate,
  endDate,
  limit,
  maxLimit,
  trendLoading,
  trendError,
  runs,
  runsLoading,
  trendData,
  onChangeFormat,
  onChangeStartDate,
  onChangeEndDate,
  onChangeLimit,
  onLoad,
}) => {
  const metricKeys = React.useMemo<string[]>(() => {
    if (!trendData?.results?.length) {
      return [];
    }

    const preferredOrder = [
      'player_runs_mae',
      'player_runs_rmse',
      'player_runs_r2',
      'player_wickets_mae',
      'player_economy_mae',
      'team_runs_mae',
      'team_wickets_mae',
      'team_extras_mae',
      'team_winner_accuracy',
    ];

    const all = new Set<string>();
    trendData.results.forEach((row) => {
      Object.keys(row.metrics || {}).forEach((k) => all.add(k));
    });

    const ordered: string[] = [];
    preferredOrder.forEach((k) => {
      if (all.has(k)) {
        ordered.push(k);
        all.delete(k);
      }
    });

    const remaining = Array.from(all).sort();
    return [...ordered, ...remaining];
  }, [trendData]);

  return (
    <SectionCard
      title="Accuracy trend"
      subtitle="Load backtest accuracy (MAE, etc.) for played matches. Filters choose which matches to include; then the API runs predictions and returns metrics."
    >
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        <strong>How to use:</strong> Set filters below (all optional), then click &quot;Load
        accuracy trend&quot;. The table shows one row per match with error metrics (e.g. runs_mae,
        wickets_mae). Leave <strong>Format</strong> as &quot;All&quot; to include every format, or
        pick one (e.g. T20) to evaluate that format only. Predictions always use the model for the
        match&apos;s own format. Prerequisites: precompute and ML artifacts must be in place.
      </Typography>
      <Stack
        direction={{ xs: 'column', sm: 'row' }}
        spacing={2}
        flexWrap="wrap"
        alignItems="flex-start"
      >
        <FormControl size="small" sx={{ minWidth: 120 }}>
          <InputLabel id="workbench-format-label">Format</InputLabel>
          <Select
            value={format}
            labelId="workbench-format-label"
            label="Format"
            onChange={(e) => onChangeFormat(e.target.value)}
          >
            <MenuItem value="">All formats</MenuItem>
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
          onChange={(e) => onChangeStartDate(e.target.value)}
          InputLabelProps={{ shrink: true }}
          sx={{ width: 160 }}
          helperText="Only matches on or after this date (YYYY-MM-DD). Leave empty for no start filter."
        />
        <TextField
          size="small"
          label="End date"
          type="date"
          value={endDate}
          onChange={(e) => onChangeEndDate(e.target.value)}
          InputLabelProps={{ shrink: true }}
          sx={{ width: 160 }}
          helperText="Only matches on or before this date (YYYY-MM-DD). Leave empty for no end filter."
        />
        <TextField
          size="small"
          label="Limit"
          type="number"
          value={limit}
          onChange={(e) => onChangeLimit(Number(e.target.value) || limit)}
          inputProps={{ min: 1, max: maxLimit }}
          sx={{ width: 90 }}
          helperText={`Max matches to evaluate (1–${maxLimit}). Fewer = faster.`}
        />
        <Button
          variant="contained"
          onClick={onLoad}
          disabled={trendLoading}
          startIcon={trendLoading ? <CircularProgress size={16} color="inherit" /> : null}
        >
          {trendLoading ? 'Loading…' : 'Load accuracy trend'}
        </Button>
      </Stack>
      <Box sx={{ mt: trendError ? 2 : 0 }}>
        <ErrorNotice error={trendError} title="Could not load the accuracy trend" />
      </Box>
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
                    <TableCell key={k}>{k}</TableCell>
                  ))}
                </TableRow>
              </TableHead>
              <TableBody>
                {trendData.results?.map((row: AccuracyTrendItem) => (
                  <TableRow key={row.match_id}>
                    <TableCell>{row.match_id}</TableCell>
                    <TableCell>{row.match_date}</TableCell>
                    <TableCell>{row.format}</TableCell>
                    <TableCell>{row.team1}</TableCell>
                    <TableCell>{row.team2}</TableCell>
                    {metricKeys.map((k) => (
                      <TableCell key={k}>
                        {formatMetricValue(k, row.metrics[k] as number | undefined)}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
          <AccuracyTrendRuns
            results={trendData.results ?? []}
            migrations={runs}
            loading={runsLoading}
          />
        </Box>
      )}
    </SectionCard>
  );
};

export default WorkbenchAccuracyTrendSection;
