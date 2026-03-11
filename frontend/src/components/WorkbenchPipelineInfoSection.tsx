import React from 'react';
import { Box, Typography, Paper } from '@mui/material';
import SectionCard from './common/SectionCard';

const WorkbenchPipelineInfoSection: React.FC = () => {
  return (
    <SectionCard
      title="Internal process: from import to prediction"
      subtitle="End-to-end pipeline: data import, precompute, export, training/auto-tune, and prediction."
    >
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        The system runs a strict sequence of steps. Each step depends on the previous; artifacts (DB
        tables, CSVs, joblib models) are produced in order and consumed by the next stage. Below is
        the full internal process.
      </Typography>

      {/* Step 1: Import */}
      <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
        <Typography variant="subtitle2" fontWeight={600} gutterBottom>
          1. Import (cricsheet-import)
        </Typography>
        <Typography variant="body2" color="text.secondary" component="div">
          <strong>What:</strong> Apply DB migrations, then ingest Cricsheet JSON (or curated CSVs)
          into the database.
          <Box component="ul" sx={{ m: 0.5, pl: 2.5 }}>
            <li>
              Source: <code>inputs.cricsheet_dir</code> (go-app config) or API body (e.g.{' '}
              <code>POST /import/cricsheet</code> with <code>dir</code>).
            </li>
            <li>
              Writes: <code>match</code>, <code>match_inning</code>, <code>match_format</code>,{' '}
              <code>batting_data</code>, <code>bowling_data</code>, <code>fielding_data</code>,{' '}
              <code>player</code>, <code>venue</code>, <code>opposition</code>, <code>season</code>,{' '}
              <code>weather_data</code>, etc.
            </li>
            <li>
              Each match has innings, player-level runs/balls/wickets, and optional weather. No
              features yet — only raw facts.
            </li>
          </Box>
          <strong>Commands:</strong> <code>make migrate</code> then{' '}
          <code>make cricsheet-import</code> (or trigger Import from Ops → Pipeline). Import must
          complete before precompute.
        </Typography>
      </Paper>

      {/* Step 2: Precompute */}
      <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
        <Typography variant="subtitle2" fontWeight={600} gutterBottom>
          2. Precompute (precompute-features)
        </Typography>
        <Typography variant="body2" color="text.secondary" component="div">
          <strong>What:</strong> Compute raw windowed statistics (mean, std, etc. over recent
          windows) and (when enabled) sequence features per player/format, as-of each snapshot date.
          <Box component="ul" sx={{ m: 0.5, pl: 2.5 }}>
            <li>Reads: batting_data, bowling_data, fielding_data, match, match_inning.</li>
            <li>
              Writes: <code>feature_raw_stats_snapshots</code> (and optionally sequence tables).
              Snapshots are keyed by player_id, format_id, as_of_date, scope (overall / venue /
              opposition).
            </li>
            <li>
              Fielding: EWM of historical catches/run_outs per player; used when no ML fielding
              model is loaded.
            </li>
          </Box>
          <strong>Commands:</strong> <code>make precompute-all-all-formats</code> (or Precompute
          from Ops → Pipeline). Prerequisite: Import done. Required before export and before any
          prediction that needs form/consistency.
        </Typography>
      </Paper>

      {/* Step 3: Export */}
      <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
        <Typography variant="subtitle2" fontWeight={600} gutterBottom>
          3. Export (export-dataset)
        </Typography>
        <Typography variant="body2" color="text.secondary" component="div">
          <strong>What:</strong> Build training rows for batting, bowling, fielding, extras, and win
          by joining match/innings data with precomputed snapshots and weather. Write CSVs and/or
          serve the same data via the training-data API.
          <Box component="ul" sx={{ m: 0.5, pl: 2.5 }}>
            <li>
              Reads: match, match_inning, batting_data, bowling_data, fielding_data,
              feature_*_snapshots, weather_data. Uses strict cutoff: only matches with{' '}
              <code>{'match_date < cutoff'}</code>.
            </li>
            <li>
              Output: <code>outputs.export_dir</code> (e.g. <code>output/go-app</code>) —{' '}
              <code>batting_encoded_all.csv</code>, <code>bowling_encoded_all.csv</code>, and when{' '}
              <code>export.split_by_format</code> is true, per-format CSVs (e.g.{' '}
              <code>batting_encoded_T20.csv</code>). Fielding/extras/win rows are not written as
              standalone CSVs by default; they are served via{' '}
              <code>GET /api/backtest/training-data?cutoff=...&amp;format=all</code>.
            </li>
            <li>
              Each row is one player-match (batting/bowling/fielding) or one match (extras/win),
              with all features and the target column.
            </li>
          </Box>
          <strong>Commands:</strong> <code>make export-dataset</code> (or Export from Ops →
          Pipeline). Prerequisite: Precompute done.
        </Typography>
      </Paper>

      {/* Step 4: Training / Auto-tune */}
      <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
        <Typography variant="subtitle2" fontWeight={600} gutterBottom>
          4. Training (train-all) and auto-tune
        </Typography>
        <Typography variant="body2" color="text.secondary" component="div">
          <strong>What:</strong> Train ML models (batting, bowling, fielding, extras, win, and
          optionally innings) from the exported CSVs or training-data API. Produces joblib artifacts
          (model + optional scaler) per model type and format. Auto-tune can search algorithms and
          hyperparameters first, then normal training reuses the tuned params (single-train
          principle).
          <Box component="ul" sx={{ m: 0.5, pl: 2.5 }}>
            <li>
              Reads: CSVs from export_dir or <code>GET /api/backtest/training-data</code>. Uses
              hyperparameters from <code>ml-service/configs/</code> (or auto-tune results).
            </li>
            <li>
              Writes: <code>output/ml-service/</code> — e.g. <code>batting_model_T20.joblib</code>,{' '}
              <code>batting_scaler_T20.joblib</code>, <code>extras_model_ODI.joblib</code>,{' '}
              <code>win_model.joblib</code>, <code>innings_model_T20.joblib</code>, etc.
            </li>
            <li>
              Optional: <code>train_combination_meta</code> — learns Ridge weights to combine
              batting/bowling/fielding scores for team selection. Output:{' '}
              <code>combination_meta.json</code>.
            </li>
            <li>
              Optional: <code>ml-auto-tune</code> — coarse-to-fine search over algorithms and
              hyperparameters. Saves best params to the go-app DB and writes tuned artifacts; normal
              training then uses those params.
            </li>
          </Box>
          <strong>Commands:</strong> <code>make ml-train-all</code> (or Train from Ops → Pipeline).
          For tuning, use <code>make ml-auto-tune MODEL=batting FORMAT=T20</code> (or trigger the
          Auto-tune pipeline step). Prerequisite: Export done.
        </Typography>
      </Paper>

      {/* Step 5: Prediction */}
      <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
        <Typography variant="subtitle2" fontWeight={600} gutterBottom>
          5. Prediction (predict)
        </Typography>
        <Typography variant="body2" color="text.secondary" component="div">
          <strong>What:</strong> Load trained models and scalers, accept a match context (teams,
          venue, date, format, weather), and produce predictions (runs, wickets, extras, win
          probability, team selection, and optionally Monte Carlo simulations over top XIs).
          <Box component="ul" sx={{ m: 0.5, pl: 2.5 }}>
            <li>
              Reads: joblib artifacts from <code>output/ml-service/</code>; precomputed snapshots
              from DB (form, consistency as-of match date); weather from DB or API.
            </li>
            <li>
              go-app calls ml-service via <code>POST /predict/batting</code>,{' '}
              <code>/predict/bowling</code>, etc. ml-service loads the correct model (per-format or
              legacy), scales features, and returns predictions.
            </li>
            <li>
              go-app aggregates player predictions into match totals, applies combination meta
              weights (if loaded), and returns the final scorecard + win probability. When enabled,
              it can also run Monte Carlo simulation for win probability and innings total
              distributions.
            </li>
          </Box>
          <strong>Commands:</strong> Use the Upcoming Match tab or{' '}
          <code>POST /api/predict/match</code> with match context. Prerequisite: Training done and
          models loaded.
        </Typography>
      </Paper>

      {/* Flow summary */}
      <Paper variant="outlined" sx={{ p: 2, bgcolor: 'grey.50' }}>
        <Typography variant="subtitle2" color="text.secondary" gutterBottom>
          Summary flow
        </Typography>
        <Typography
          variant="body2"
          fontFamily="monospace"
          component="div"
          sx={{ fontSize: '0.85rem' }}
        >
          Import → Precompute → Export → Train (batting, bowling, fielding, extras, win) [→
          optional: train_combination_meta] → Prediction
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
          Ops Status → Pipeline shows each step as runnable only after the previous completed.
          Accuracy trend and walk-forward in this Workbench consume the result of this pipeline.
        </Typography>
      </Paper>
    </SectionCard>
  );
};

export default WorkbenchPipelineInfoSection;
