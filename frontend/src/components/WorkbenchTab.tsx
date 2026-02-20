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
  Accordion,
  AccordionSummary,
  AccordionDetails,
  Chip,
} from '@mui/material';
import UploadFileIcon from '@mui/icons-material/UploadFile';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ArrowForwardIcon from '@mui/icons-material/ArrowForward';
import SectionCard from './common/SectionCard';
import { api } from '../api';
import type {
  AccuracyTrendResponse,
  AccuracyTrendFilters,
  AccuracyTrendItem,
  WalkForwardRegistry,
  WalkForwardWindowEntry,
} from '../types';

// Canonical feature and output lists per model (aligned with configs/feature_vectors.json and train_* scripts).
// artifactsPattern: per-format files (FMT = T20, ODI, TEST, T20I) and legacy (unified) files.
const MODEL_FEATURES: Record<
  string,
  {
    features: string[];
    outputs: string[];
    level: 'player' | 'match' | 'meta';
    note?: string;
    artifactsPattern: { perFormat: string; legacy: string };
    hasScaler?: boolean;
  }
> = {
  batting: {
    level: 'player',
    hasScaler: true,
    artifactsPattern: {
      perFormat: 'batting_scaler_<FMT>.joblib + batting_model_<FMT>.joblib',
      legacy: 'batting_scaler.joblib + batting_model.joblib',
    },
    features: [
      'batting_consistency',
      'batting_form',
      'batting_form_short',
      'batting_form_long',
      'batting_momentum',
      'batting_temp',
      'batting_wind',
      'batting_rain',
      'batting_humidity',
      'batting_cloud',
      'batting_pressure',
      'batting_viscosity',
      'batting_inning',
      'batting_session',
      'toss',
      'venue',
      'opposition',
      'season',
      'bat_prev_sr',
      'bat_prev_out_rate',
      'bat_window_sr_12_pp',
      'bat_window_boundary_rate_12_pp',
      'bat_entry_sr_1_6',
      'bat_set_sr_13_30',
      'bat_react_after_dot_sr',
      'bat_after_k_dots_boundary_p_k2',
    ],
    outputs: ['runs_scored', 'balls_faced', 'fours_scored', 'sixes_scored', 'batting_position', 'strike_rate'],
    note: 'Player-level; same feature families used for match-level models. Prediction uses per-format model when format (e.g. T20) is provided and loaded; otherwise legacy.',
  },
  bowling: {
    level: 'player',
    hasScaler: true,
    artifactsPattern: {
      perFormat: 'bowling_scaler_<FMT>.joblib + bowling_model_<FMT>.joblib',
      legacy: 'bowling_scaler.joblib + bowling_model.joblib',
    },
    features: [
      'bowling_consistency',
      'bowling_form',
      'bowling_form_short',
      'bowling_form_long',
      'bowling_momentum',
      'bowling_temp',
      'bowling_wind',
      'bowling_rain',
      'bowling_humidity',
      'bowling_cloud',
      'bowling_pressure',
      'bowling_viscosity',
      'batting_inning',
      'bowling_session',
      'toss',
      'bowling_venue',
      'bowling_opposition',
      'season',
      'bowl_prev_wkt_rate',
      'bowl_window_econ_24_death',
      'bowl_window_wkt_rate_24_death',
      'bowl_extras_wide_rate_pp',
      'bowl_react_after_boundary_wkt_rate_next',
      'bowl_spell_first_over_wkt_rate',
      'bowl_over_ball1_wkt_rate',
      'bowl_over_ball6_wkt_rate',
    ],
    outputs: ['runs_conceded', 'deliveries', 'wickets_taken', 'economy'],
    note: 'Player-level; aggregates feed into extras and win. Per-format and legacy same as batting.',
  },
  fielding: {
    level: 'player',
    hasScaler: true,
    artifactsPattern: {
      perFormat: 'fielding_scaler_<FMT>.joblib + fielding_model_<FMT>.joblib',
      legacy: 'fielding_scaler.joblib + fielding_model.joblib',
    },
    features: [
      'fielding_consistency',
      'fielding_form',
      'fielding_temp',
      'fielding_wind',
      'fielding_rain',
      'fielding_humidity',
      'fielding_cloud',
      'fielding_pressure',
      'fielding_viscosity',
      'inning',
      'toss',
      'fielding_venue',
      'fielding_opposition',
      'season_id',
    ],
    outputs: ['catches', 'run_outs', 'stumpings'],
    note: 'Player-level; combined with batting/bowling for team selection. Per-format and legacy same as batting.',
  },
  extras: {
    level: 'match',
    hasScaler: false,
    artifactsPattern: {
      perFormat: 'extras_model_<FMT>.joblib',
      legacy: 'extras_model.joblib',
    },
    features: [
      'format_id',
      'venue_id',
      'season_id',
      'temp',
      'wind',
      'rain',
      'humidity',
      'cloud',
      'pressure',
      'viscosity',
      'bat_consistency_sum',
      'bowl_consistency_sum',
      'bat_form_sum',
      'bowl_form_sum',
    ],
    outputs: ['total_extras'],
    note: 'Match-level; uses same weather + aggregates of player consistency/form from batting/bowling snapshot data. One model per format or legacy.',
  },
  win: {
    level: 'match',
    hasScaler: false,
    artifactsPattern: {
      perFormat: 'win_model_<FMT>.joblib',
      legacy: 'win_model.joblib',
    },
    features: [
      'format_id',
      'venue_id',
      'team1_opposition_id',
      'team2_opposition_id',
      'toss_winner_opposition_id',
      'temp',
      'wind',
      'rain',
      'humidity',
      'cloud',
      'pressure',
      'viscosity',
      'team1_bat_consistency_sum',
      'team1_bowl_consistency_sum',
      'team2_bat_consistency_sum',
      'team2_bowl_consistency_sum',
      'team1_bat_form_sum',
      'team1_bowl_form_sum',
      'team2_bat_form_sum',
      'team2_bowl_form_sum',
    ],
    outputs: ['team1_win_probability'],
    note: 'Match-level; team1 = batting first, team2 = bowling first. Same feature families as batting/bowling/fielding. One model per format or legacy.',
  },
  combination_meta: {
    level: 'meta',
    hasScaler: false,
    artifactsPattern: {
      perFormat: 'Optional per-format weights in combination_meta.json',
      legacy: 'combination_meta.json (unified weights)',
    },
    features: ['bat_score', 'bowl_score', 'field_score', 'is_keeper', 'format (one-hot)'],
    outputs: ['score_weights (Ridge coefficients for team selection)'],
    note: 'Optional. Learns weights to combine batting/bowling/fielding scores from backtest outcomes. Uses CSV with bat_score, bowl_score, field_score, is_keeper, format, target. Output is JSON (not joblib); go-app can load via selection.meta_model_path.',
  },
};

const DEFAULT_LIMIT = 100;
const MAX_LIMIT = 500;

const WorkbenchTab: React.FC = () => {
  const [format, setFormat] = useState<string>('');
  const [predictionModel, setPredictionModel] = useState<'format' | 'unified'>('format');
  const [startDate, setStartDate] = useState<string>('');
  const [endDate, setEndDate] = useState<string>('');
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
        order: 'asc',
        limit: Math.min(Math.max(1, limit), MAX_LIMIT),
        cache: 'read',
        use_unified_model: predictionModel === 'unified',
      };
      const data = await api.accuracyTrend(filters);
      setTrendData(data);
    } catch (e) {
      setTrendError(e instanceof Error ? e.message : String(e));
      setTrendData(null);
    } finally {
      setTrendLoading(false);
    }
  }, [format, startDate, endDate, limit, predictionModel]);

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
      <Alert severity="info" sx={{ mb: 1 }}>
        <Typography variant="subtitle2" gutterBottom>
          What is the Workbench?
        </Typography>
        <Typography variant="body2" component="span">
          The Workbench lets you inspect how well the ML models predict real match outcomes. Use{' '}
          <strong>Accuracy trend</strong> to load backtest results (per-match MAE and aggregates),
          and <strong>Walk-forward registry</strong> to view results from the walk-forward pipeline
          (train → predict next window → score). Choose <strong>Prediction model</strong>:
          format-specific (model for the selected format) or <strong>Unified</strong> (legacy
          all-formats model).
        </Typography>
      </Alert>

      <SectionCard
        title="Internal process: from import to prediction"
        subtitle="End-to-end pipeline: data import, precompute, export, training, and prediction."
      >
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          The system runs a strict sequence of steps. Each step depends on the previous; artifacts (DB tables, CSVs, joblib models) are produced in order and consumed by the next stage. Below is the full internal process.
        </Typography>

        {/* Step 1: Import */}
        <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
          <Typography variant="subtitle2" fontWeight={600} gutterBottom>
            1. Import (cricsheet-import)
          </Typography>
          <Typography variant="body2" color="text.secondary" component="div">
            <strong>What:</strong> Apply DB migrations, then ingest Cricsheet JSON (or curated CSVs) into the database.
            <Box component="ul" sx={{ m: 0.5, pl: 2.5 }}>
              <li>Source: <code>inputs.cricsheet_dir</code> (go-app config) or API body (e.g. <code>POST /import/cricsheet</code> with <code>dir</code>).</li>
              <li>Writes: <code>match</code>, <code>match_inning</code>, <code>match_format</code>, <code>batting_data</code>, <code>bowling_data</code>, <code>fielding_data</code>, <code>player</code>, <code>venue</code>, <code>opposition</code>, <code>season</code>, <code>weather_data</code>, etc.</li>
              <li>Each match has innings, player-level runs/balls/wickets, and optional weather. No features yet — only raw facts.</li>
            </Box>
            <strong>Commands:</strong> <code>make migrate</code> then <code>make cricsheet-import</code> (or trigger Import from Ops → Pipeline). Import must complete before precompute.
          </Typography>
        </Paper>

        {/* Step 2: Precompute */}
        <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
          <Typography variant="subtitle2" fontWeight={600} gutterBottom>
            2. Precompute (precompute-features)
          </Typography>
          <Typography variant="body2" color="text.secondary" component="div">
            <strong>What:</strong> Compute form, consistency, venue/opposition effects, and (when enabled) sequence features per player/format, as-of each snapshot date.
            <Box component="ul" sx={{ m: 0.5, pl: 2.5 }}>
              <li>Reads: batting_data, bowling_data, fielding_data, match, match_inning; uses <code>features.ewm_alpha</code>, <code>consistency_last_n</code>, <code>momentum_last_n</code>, etc. from go-app config.</li>
              <li>Writes: <code>feature_form_snapshots</code>, <code>feature_consistency_snapshots</code> (and optionally sequence tables). Snapshots are keyed by player_id, format_id, as_of_date, scope (overall / venue / opposition).</li>
              <li>Fielding: EWM of historical catches/run_outs per player; used when no ML fielding model is loaded.</li>
            </Box>
            <strong>Commands:</strong> <code>make precompute-all-all-formats</code> (or Precompute from Ops → Pipeline). Prerequisite: Import done. Required before export and before any prediction that needs form/consistency.
          </Typography>
        </Paper>

        {/* Step 3: Export */}
        <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
          <Typography variant="subtitle2" fontWeight={600} gutterBottom>
            3. Export (export-dataset)
          </Typography>
          <Typography variant="body2" color="text.secondary" component="div">
            <strong>What:</strong> Build training rows for batting, bowling, fielding, extras, and win by joining match/innings data with precomputed snapshots and weather. Write CSVs and/or serve the same data via the training-data API.
            <Box component="ul" sx={{ m: 0.5, pl: 2.5 }}>
              <li>Reads: match, match_inning, batting_data, bowling_data, fielding_data, feature_*_snapshots, weather_data. Uses strict cutoff: only matches with <code>{'match_date < cutoff'}</code>.</li>
              <li>Output: <code>outputs.export_dir</code> (e.g. <code>output/go-app</code>) — <code>batting_encoded_all.csv</code>, <code>bowling_encoded_all.csv</code>, and when <code>export.split_by_format</code> is true, per-format CSVs (e.g. <code>batting_encoded_T20.csv</code>). Fielding/extras/win rows are not written as standalone CSVs by default; they are served via <code>GET /api/backtest/training-data?cutoff=...&amp;format=all</code>.</li>
              <li>Same feature computation as at prediction: form, consistency, venue, opposition, weather, sequence columns from <code>configs/feature_vectors.json</code>.</li>
            </Box>
            <strong>Commands:</strong> <code>make export-dataset</code> (or Export from Pipeline). Prerequisite: Precompute done. Batting/bowling training read from these CSVs; fielding/extras/win training typically use the API with a cutoff.
          </Typography>
        </Paper>

        {/* Step 4: Training */}
        <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
          <Typography variant="subtitle2" fontWeight={600} gutterBottom>
            4. Training (train-batting, train-bowling, train-fielding, train-extras, train-win)
          </Typography>
          <Typography variant="body2" color="text.secondary" component="div">
            <strong>What:</strong> ML service trains per-format and unified (legacy) models; writes scaler + model (or model only) joblib files to the artifacts directory.
            <Box component="ul" sx={{ m: 0.5, pl: 2.5 }}>
              <li><strong>Batting / Bowling:</strong> Read from exported CSVs (or <code>--from-api --cutoff</code>). Fit StandardScaler on X, train regressor (e.g. RandomForest), save <code>batting_scaler_&lt;FMT&gt;.joblib</code>, <code>batting_model_&lt;FMT&gt;.joblib</code> and legacy unsuffixed files.</li>
              <li><strong>Fielding / Extras / Win:</strong> Fetch training data from go-app <code>GET /api/backtest/training-data?format=all&amp;cutoff=...</code>. Train per format and one legacy model; save to same artifacts dir (fielding: scaler+model; extras/win: model only).</li>
              <li><strong>Optional — Combination meta:</strong> <code>train_combination_meta</code> reads a CSV of bat_score, bowl_score, field_score, is_keeper, format, target; outputs JSON weights for team selection. Run after train_win when contributions CSV is available.</li>
            </Box>
            <strong>Commands:</strong> <code>make train-batting</code>, <code>make train-bowling</code>, <code>make train-fielding CUTOFF=...</code>, <code>make train-extras</code>, <code>make train-win</code> (or trigger from Ops → Pipeline). Prerequisite: Export done; for fielding/extras/win, GO_APP_URL must point at go-app. Artifacts are loaded by the ML service on startup or reload.
          </Typography>
        </Paper>

        {/* Step 5: Prediction */}
        <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
          <Typography variant="subtitle2" fontWeight={600} gutterBottom>
            5. Prediction (backtest, team selection, scorecard)
          </Typography>
          <Typography variant="body2" color="text.secondary" component="div">
            <strong>What:</strong> go-app builds a feature map per player (same logic as export: form, consistency, venue, opposition, weather, sequence from snapshots as-of cutoff). Calls ML service to get player and match-level predictions; aggregates to match outcome and optional team selection.
            <Box component="ul" sx={{ m: 0.5, pl: 2.5 }}>
              <li><strong>Feature map:</strong> For each player and format, go-app computes or looks up form, consistency, venue effect, opposition effect, season, optional weather. Sequence features (e.g. bat_prev_sr) come from precompute/seqcalc when available; otherwise 0. Same keys and order as <code>configs/feature_vectors.json</code>.</li>
              <li><strong>Player predictions:</strong> go-app sends feature vectors to ML <code>POST /predict/batting</code>, <code>POST /predict/bowling</code>, <code>POST /predict/fielding</code>. ML uses per-format model when request includes format and that model is loaded; else legacy. Returns runs, balls, wickets, economy, catches, run_outs, etc.</li>
              <li><strong>Match-level:</strong> Extras: from <code>POST /predict/extras</code> (if model loaded) with match-level features, or from DB historical average (<code>GetAverageExtrasForFormat</code>). Win: from <code>POST /predict/win</code> with team/match features, or by comparing innings totals (sum of player runs + extras).</li>
              <li><strong>Aggregates:</strong> Predicted innings total = sum of selected XI batting runs + extras. Winner = higher total or win model output. Team selection: scores from batting/bowling/fielding predictions, optional combination-meta weights, constraints (min bowlers, keeper); optimizer or greedy selection picks XI.</li>
            </Box>
            <strong>APIs:</strong> Backtest: <code>GET /api/backtest/accuracy-trend</code> (runs predictions and returns MAE, etc.). Team selection: <code>POST /api/predict/team-selection</code> with format, venue, teams, pool; response includes selected XI and predicted stats. All prediction uses data strictly before the requested cutoff so there is no future leakage.
          </Typography>
        </Paper>

        {/* Flow summary */}
        <Paper variant="outlined" sx={{ p: 2, bgcolor: 'grey.50' }}>
          <Typography variant="subtitle2" color="text.secondary" gutterBottom>
            Pipeline order summary
          </Typography>
          <Typography variant="body2" fontFamily="monospace" component="div" sx={{ fontSize: '0.85rem' }}>
            Import → Precompute → Export → Train (batting, bowling, fielding, extras, win) [→ optional: train_combination_meta] → Prediction
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
            Ops Status → Pipeline shows each step as runnable only after the previous completed. Accuracy trend and walk-forward in this Workbench consume the result of this pipeline.
          </Typography>
        </Paper>
      </SectionCard>

      <SectionCard
        title="Accuracy trend"
        subtitle="Load backtest accuracy (MAE, etc.) for played matches. Filters choose which matches to include; then the API runs predictions and returns metrics."
      >
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          <strong>How to use:</strong> Set filters below (all optional), then click &quot;Load
          accuracy trend&quot;. Use <strong>Prediction model</strong> to compare format-specific
          models vs the unified (legacy) model. The table shows one row per match with error metrics
          (e.g. runs_mae, wickets_mae). Leave <strong>Format</strong> as &quot;All&quot; to include
          every format, or pick one (e.g. T20) to evaluate that format only. Prerequisites:
          precompute and ML artifacts must be in place.
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
              onChange={(e) => setFormat(e.target.value)}
            >
              <MenuItem value="">All formats</MenuItem>
              {availableFormats.map((f) => (
                <MenuItem key={f} value={f}>
                  {f}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <FormControl size="small" sx={{ minWidth: 200 }}>
            <InputLabel id="workbench-model-label">Prediction model</InputLabel>
            <Select
              value={predictionModel}
              labelId="workbench-model-label"
              label="Prediction model"
              onChange={(e) => setPredictionModel(e.target.value as 'format' | 'unified')}
            >
              <MenuItem value="format">Format-specific (model for selected format)</MenuItem>
              <MenuItem value="unified">Unified (all-formats / legacy model)</MenuItem>
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
            helperText="Only matches on or after this date (YYYY-MM-DD). Leave empty for no start filter."
          />
          <TextField
            size="small"
            label="End date"
            type="date"
            value={endDate}
            onChange={(e) => setEndDate(e.target.value)}
            InputLabelProps={{ shrink: true }}
            sx={{ width: 160 }}
            helperText="Only matches on or before this date (YYYY-MM-DD). Leave empty for no end filter."
          />
          <TextField
            size="small"
            label="Limit"
            type="number"
            value={limit}
            onChange={(e) => setLimit(Number(e.target.value) || DEFAULT_LIMIT)}
            inputProps={{ min: 1, max: MAX_LIMIT }}
            sx={{ width: 90 }}
            helperText={`Max matches to evaluate (1–${MAX_LIMIT}). Fewer = faster.`}
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
        subtitle="Upload a walk-forward registry JSON to view how well the model generalizes over time."
      >
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
          <strong>What it is:</strong> Walk-forward evaluation tests whether your model stays
          accurate as time moves forward. For each &quot;window&quot;, the pipeline trains on data
          only
          <em> before </em> a cutoff date, then predicts the next X matches (holdout), and compares
          predictions to actual results. The registry file records each window (model, format,
          cutoff, metrics like MAE). This helps you spot if the model degrades on newer matches.
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
          <code>walk_forward_registry.json</code> to the ML service output directory. Upload that
          file below.
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          <strong>Expected file shape:</strong> JSON with <code>run_id</code> (string) and{' '}
          <code>windows</code> (array). Each window has <code>model_type</code>, <code>format</code>
          , <code>cutoff_trained_before</code>, <code>window_x</code>, <code>metrics</code> (e.g.{' '}
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
        title="Model features & interconnection"
        subtitle="All model types, per-format vs unified artifacts, features, outputs, and how they connect."
      >
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          The pipeline trains <strong>six model types</strong>: <strong>batting</strong>, <strong>bowling</strong>, <strong>fielding</strong> (player-level),
          <strong> extras</strong> and <strong>win</strong> (match-level), and optionally <strong>combination meta</strong> (weights for team selection).
          All share the same feature families (context, form, consistency, venue, opposition, weather). Match-level models use aggregates of player
          features so team composition influences extras and win probability.
        </Typography>

        {/* Per-format vs unified (legacy) */}
        <Paper variant="outlined" sx={{ p: 2, mb: 2, bgcolor: 'primary.50' }}>
          <Typography variant="subtitle2" fontWeight={600} gutterBottom>
            Per-format and unified (legacy) models
          </Typography>
          <Typography variant="body2" color="text.secondary" component="span">
            For each model type we train <strong>both</strong>:
          </Typography>
          <Box component="ul" sx={{ m: 0.5, pl: 2.5 }}>
            <li>
              <strong>Per-format:</strong> one model (and scaler where applicable) per format. Artifacts are named with the format code, e.g.{' '}
              <code>batting_scaler_T20.joblib</code>, <code>batting_model_T20.joblib</code>. Typical formats: <strong>T20</strong>, <strong>ODI</strong>,{' '}
              <strong>TEST</strong>, <strong>T20I</strong>.
            </li>
            <li>
              <strong>Unified (legacy):</strong> one model trained on all formats, e.g. <code>batting_scaler.joblib</code>, <code>batting_model.joblib</code>.
              Used when no per-format model is loaded or when the request does not specify a format.
            </li>
          </Box>
          <Typography variant="body2" color="text.secondary">
            At prediction time: if the request includes a format (e.g. T20) and that format&apos;s model is loaded, it is used; otherwise the legacy
            model is used. The Workbench &quot;Prediction model&quot; selector above lets you compare <strong>format-specific</strong> vs{' '}
            <strong>unified</strong> for accuracy trend.
          </Typography>
        </Paper>

        {/* Flow diagram: Player models → Match models → Outcome */}
        <Paper variant="outlined" sx={{ p: 2, mb: 3, bgcolor: 'grey.50' }}>
          <Typography variant="subtitle2" color="text.secondary" gutterBottom>
            Prediction flow
          </Typography>
          <Stack
            direction={{ xs: 'column', sm: 'row' }}
            spacing={1}
            alignItems="center"
            flexWrap="wrap"
            useFlexGap
          >
            <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
              {(['batting', 'bowling', 'fielding'] as const).map((key) => (
                <Chip
                  key={key}
                  label={key}
                  size="small"
                  color="primary"
                  variant="outlined"
                  sx={{ textTransform: 'capitalize' }}
                />
              ))}
            </Stack>
            <ArrowForwardIcon sx={{ color: 'action.active', fontSize: 20 }} />
            <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
              {(['extras', 'win'] as const).map((key) => (
                <Chip
                  key={key}
                  label={key}
                  size="small"
                  color="secondary"
                  variant="outlined"
                  sx={{ textTransform: 'capitalize' }}
                />
              ))}
            </Stack>
            <ArrowForwardIcon sx={{ color: 'action.active', fontSize: 20 }} />
            <Chip label="Match outcome (totals + winner)" size="small" variant="filled" />
          </Stack>
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
            Optional: combination meta weights combine batting/bowling/fielding scores for team selection.
          </Typography>
        </Paper>

        {/* Per-model features, outputs, and artifacts */}
        <Typography variant="subtitle2" color="text.secondary" gutterBottom>
          Features, outputs, and artifacts by model
        </Typography>
        {(['batting', 'bowling', 'fielding', 'extras', 'win', 'combination_meta'] as const).map((key) => {
          const m = MODEL_FEATURES[key];
          if (!m) return null;
          return (
            <Accordion key={key} defaultExpanded={key === 'batting'} disableGutters sx={{ '&:before': { display: 'none' } }}>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Stack direction="row" alignItems="center" spacing={1} flexWrap="wrap">
                  <Typography sx={{ textTransform: 'capitalize', fontWeight: 600 }}>
                    {key.replace('_', ' ')}
                  </Typography>
                  <Chip label={m.level} size="small" variant="outlined" sx={{ fontSize: '0.7rem' }} />
                  {m.hasScaler !== undefined && (
                    <Chip
                      label={m.hasScaler ? 'scaler + model' : 'model only'}
                      size="small"
                      variant="outlined"
                      sx={{ fontSize: '0.65rem' }}
                    />
                  )}
                  <Typography variant="caption" color="text.secondary">
                    {m.features.length} features → {m.outputs.length} output{m.outputs.length !== 1 ? 's' : ''}
                  </Typography>
                </Stack>
              </AccordionSummary>
              <AccordionDetails sx={{ pt: 0 }}>
                {m.note && (
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                    {m.note}
                  </Typography>
                )}
                <Box sx={{ mb: 1.5 }}>
                  <Typography variant="caption" fontWeight={600} color="text.secondary">
                    Artifacts (per-format &amp; legacy)
                  </Typography>
                  <Box
                    component="ul"
                    sx={{ m: 0, pl: 2, fontSize: '0.75rem', color: 'text.secondary' }}
                  >
                    <li>
                      <strong>Per-format:</strong> {m.artifactsPattern.perFormat} — FMT = T20, ODI, TEST, T20I, etc.
                    </li>
                    <li>
                      <strong>Legacy (unified):</strong> {m.artifactsPattern.legacy}
                    </li>
                  </Box>
                </Box>
                <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}>
                  <Box sx={{ flex: 1 }}>
                    <Typography variant="caption" fontWeight={600} color="text.secondary">
                      Features
                    </Typography>
                    <Box
                      component="ul"
                      sx={{
                        m: 0,
                        pl: 2,
                        fontSize: '0.8rem',
                        fontFamily: 'monospace',
                        maxHeight: 160,
                        overflow: 'auto',
                      }}
                    >
                      {m.features.map((f) => (
                        <li key={f}>{f}</li>
                      ))}
                    </Box>
                  </Box>
                  <Box sx={{ flex: 0, minWidth: 160 }}>
                    <Typography variant="caption" fontWeight={600} color="text.secondary">
                      Outputs
                    </Typography>
                    <Box component="ul" sx={{ m: 0, pl: 2, fontSize: '0.8rem', fontFamily: 'monospace' }}>
                      {m.outputs.map((o) => (
                        <li key={o}>{o}</li>
                      ))}
                    </Box>
                  </Box>
                </Stack>
              </AccordionDetails>
            </Accordion>
          );
        })}
      </SectionCard>

      <SectionCard
        title="Commands & docs"
        subtitle="Reference: how to generate the data you view in the sections above."
      >
        <Typography
          variant="body2"
          component="div"
          sx={{ '& code': { bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 } }}
        >
          <Box component="ul" sx={{ m: 0, pl: 2.5 }}>
            <li>
              <strong>Accuracy trend</strong> — Data comes from the go-app backtest API. Ensure
              precompute and ML artifacts are in place, then use the filters in the first section
              and click &quot;Load accuracy trend&quot;.
            </li>
            <li>
              <strong>Walk-forward</strong> — From repo root:{' '}
              <code>
                make walk-forward INITIAL_CUTOFF=2020-01-01T00:00:00Z WINDOW_X=50 WALK_FORMAT=T20
              </code>
              . The registry is written to the ML service output dir; upload it in the section
              above.
            </li>
            <li>
              <strong>Auto-tune</strong> — To search for better hyperparameters:{' '}
              <code>make ml-auto-tune MODEL=batting FORMAT=T20</code>. See{' '}
              <code>docs/ml-and-training.md</code> (walk-forward and auto-tune) in the repo.
            </li>
          </Box>
        </Typography>
      </SectionCard>
    </Box>
  );
};

export default WorkbenchTab;
