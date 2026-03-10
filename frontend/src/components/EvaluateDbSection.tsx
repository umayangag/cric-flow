import React from 'react';
import {
  Box,
  Button,
  FormControl,
  Grid,
  InputLabel,
  LinearProgress,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,
  MenuItem,
  Paper,
  Select,
  Stack,
  Typography,
  Alert,
  CircularProgress,
  TextField,
} from '@mui/material';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import Autocomplete, { createFilterOptions } from '@mui/material/Autocomplete';
import { accentGradient } from '../theme';
import type { BacktestCandidate, BacktestEvaluateResponse, MatchScorecardResponse } from '../types';
import type { EvaluationStep } from '../hooks/useEvaluateDb';
import CandidatesTable from './CandidatesTable';
import EvaluationResults from './EvaluationResults';
import MatchScorecard from './MatchScorecard';

const filter = createFilterOptions<string>();

/** Renders job mode line (unified/format model · latest/strict) when at least one flag is non-null. */
function JobModeLine({
  jobUseUnifiedModel,
  jobUseLatestModel,
  prefix,
  sx,
}: {
  jobUseUnifiedModel: boolean | null;
  jobUseLatestModel: boolean | null;
  prefix: string;
  sx?: object;
}) {
  if (jobUseUnifiedModel === null && jobUseLatestModel === null) return null;
  return (
    <Typography variant="body2" color="text.secondary" sx={sx}>
      {prefix}
      {jobUseUnifiedModel === null
        ? '—'
        : jobUseUnifiedModel
          ? 'Unified (all-formats) model'
          : 'Format-specific model'}
      {' · '}
      {jobUseLatestModel === null
        ? '—'
        : jobUseLatestModel
          ? 'Latest model'
          : 'Strict temporal cutoff'}
    </Typography>
  );
}

/** Shared filter for Team 1/Team 2 Autocomplete: show all when empty, require ≥3 chars when typing. */
function teamFilterOptions(options: string[], params: Parameters<typeof filter>[1]): string[] {
  const filtered = filter(options, params);
  if (params.inputValue === '') return filtered;
  if (params.inputValue.length < 3) return [];
  return filtered;
}

export interface EvaluateDbSectionProps {
  // Form inputs
  format: string;
  onFormatChange: (v: string) => void;
  team1: string;
  onTeam1Change: (v: string) => void;
  team2: string;
  onTeam2Change: (v: string) => void;

  // Options
  availableFormats: string[];
  availableTeam1s: string[];
  availableTeam2s: string[];

  // UI state
  loading: boolean;
  error: string | null;
  statusMessage: string;

  // Model settings
  predictionModel: 'format' | 'unified';
  onPredictionModelChange: (v: 'format' | 'unified') => void;

  // Backtest data
  candidates: BacktestCandidate[];
  selectedMatchId: number | null;
  onSelectMatch: (v: number | null) => void;
  evaluationResult: BacktestEvaluateResponse | null;

  // Scorecard
  scorecard: MatchScorecardResponse | null;
  scorecardLoading: boolean;
  scorecardError: string | null;

  // Evaluate job
  currentJobId: string | null;
  evaluating: boolean;
  evaluationSteps: EvaluationStep[];

  // Evaluate job mode (from backend flags)
  jobUseUnifiedModel: boolean | null;
  jobUseLatestModel: boolean | null;

  // Derived
  canLoad: boolean;
  canEvaluate: boolean;

  // Actions
  onResetOutputs: () => void;
  onLoadCandidates: () => void;
  onEvaluateSelectedMatch: () => void;
}

export const EvaluateDbSection: React.FC<EvaluateDbSectionProps> = ({
  format,
  onFormatChange,
  team1,
  onTeam1Change,
  team2,
  onTeam2Change,
  availableFormats,
  availableTeam1s,
  availableTeam2s,
  loading,
  error,
  statusMessage,
  predictionModel,
  onPredictionModelChange,
  candidates,
  selectedMatchId,
  onSelectMatch,
  evaluationResult,
  scorecard,
  scorecardLoading,
  scorecardError,
  currentJobId,
  evaluating,
  evaluationSteps,
  jobUseUnifiedModel,
  jobUseLatestModel,
  canLoad,
  canEvaluate,
  onResetOutputs,
  onLoadCandidates,
  onEvaluateSelectedMatch,
}) => (
  <Stack spacing={3} sx={{ mt: 2 }}>
    <Typography variant="body1">
      Evaluate historical matches by predicting for actual players and comparing predictions vs
      actuals. Uses the latest loaded model (fast, good for QA).
    </Typography>
    <Alert severity="info" sx={{ maxWidth: 560 }}>
      Strict cutoff and train-on-the-fly are disabled to avoid high CPU/RAM usage. Pre-trained
      artifacts must be loaded for your format (e.g. T20I). Use Latest model only.
    </Alert>

    <Stack direction={{ xs: 'column', md: 'row' }} spacing={2} alignItems="center">
      <FormControl fullWidth size="small">
        <InputLabel id="format-select-label">Format</InputLabel>
        <Select
          labelId="format-select-label"
          value={format}
          label="Format"
          onChange={(e) => {
            onFormatChange(e.target.value);
            onResetOutputs();
          }}
        >
          <MenuItem value="">Select Format</MenuItem>
          {availableFormats.map((f) => (
            <MenuItem key={f} value={f}>
              {f}
            </MenuItem>
          ))}
        </Select>
      </FormControl>

      <Autocomplete
        fullWidth
        size="small"
        disableClearable
        options={(availableTeam1s || []).filter((t) => t !== team2)}
        value={(team1 || null) as string | undefined}
        onChange={(_e, newValue) => {
          onTeam1Change(newValue ?? '');
          if (newValue) onResetOutputs();
        }}
        filterOptions={teamFilterOptions}
        renderInput={(params) => <TextField {...params} label="Team 1" />}
        noOptionsText={team1 ? 'No matching teams' : 'Type to search or select from dropdown'}
      />

      <Autocomplete
        fullWidth
        size="small"
        disableClearable
        options={(availableTeam2s || []).filter((t) => t !== team1)}
        value={(team2 || null) as string | undefined}
        onChange={(_e, newValue) => {
          onTeam2Change(newValue ?? '');
          if (newValue) onResetOutputs();
        }}
        filterOptions={teamFilterOptions}
        renderInput={(params) => <TextField {...params} label="Team 2" />}
        noOptionsText={team2 ? 'No matching teams' : 'Type to search or select from dropdown'}
      />

      <Button
        variant="contained"
        onClick={onLoadCandidates}
        disabled={!canLoad}
        sx={{ minWidth: 180, height: 40 }}
      >
        {loading && !candidates.length ? <CircularProgress size={20} sx={{ mr: 1 }} /> : null}
        Load Matches
      </Button>
    </Stack>

    {statusMessage && <Alert severity="info">{statusMessage}</Alert>}
    {error && <Alert severity="error">{error}</Alert>}

    {/* Current evaluation (restored after refresh or in progress) */}
    {currentJobId && (
      <Paper
        variant="outlined"
        sx={{
          p: 2,
          bgcolor: 'action.hover',
          borderColor: 'divider',
          borderWidth: 1,
        }}
        component="section"
        aria-label="current-evaluation"
      >
        <Typography variant="subtitle2" color="text.secondary" gutterBottom>
          Current evaluation
        </Typography>
        <Typography variant="body2">
          {format} — {team1} vs {team2}
          {selectedMatchId != null && ` · Match ${selectedMatchId}`}
          {evaluating && ' · Running (progress below)'}
          {evaluationResult && !evaluating && ' · Complete (result below)'}
          {error && !evaluating && ' · Failed (see error above)'}
        </Typography>
        <JobModeLine
          jobUseUnifiedModel={jobUseUnifiedModel}
          jobUseLatestModel={jobUseLatestModel}
          prefix="Mode: "
          sx={{ mt: 0.5 }}
        />
      </Paper>
    )}

    {/* Candidates */}
    <Box component="section" aria-label="candidates-section">
      <Typography variant="h6" gutterBottom>
        Candidates
      </Typography>
      <CandidatesTable
        candidates={candidates}
        selectedMatchId={selectedMatchId}
        onSelectMatch={onSelectMatch}
      />
      {selectedMatchId != null &&
        (scorecard ||
          scorecardLoading ||
          scorecardError ||
          evaluationResult?.predicted_scorecard) && (
          <Box sx={{ mt: 2 }}>
            <Typography variant="subtitle1" fontWeight="bold" gutterBottom>
              Scorecard: Actual vs Predicted
            </Typography>
            <Grid container spacing={3}>
              <Grid item xs={12} md={6}>
                <MatchScorecard
                  scorecard={scorecard}
                  loading={scorecardLoading}
                  error={scorecardError}
                  title="Actual"
                  subtitle="Actual match result from database"
                />
              </Grid>
              <Grid item xs={12} md={6}>
                <MatchScorecard
                  scorecard={evaluationResult?.predicted_scorecard ?? null}
                  loading={evaluating}
                  error={null}
                  title="Predicted"
                  subtitle={
                    evaluationResult
                      ? String(evaluationResult.filters.model_mode) === 'latest'
                        ? 'ML prediction using the latest model'
                        : 'ML prediction (model trained before match date)'
                      : 'Run evaluation to see predicted scorecard'
                  }
                />
              </Grid>
            </Grid>
          </Box>
        )}
      <Box sx={{ mt: 2, display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', gap: 2 }}>
        <FormControl size="small" sx={{ minWidth: 260 }}>
          <InputLabel id="eval-db-temporal-label">Model temporal mode</InputLabel>
          <Select
            labelId="eval-db-temporal-label"
            value="latest"
            label="Model temporal mode"
            disabled
          >
            <MenuItem value="latest">Latest model only</MenuItem>
          </Select>
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ display: 'block', mt: 0.5, maxWidth: 340 }}
          >
            Latest: pre-loaded artifacts. Strict: train-on-the-fly with data before match.
          </Typography>
        </FormControl>
        <FormControl size="small" sx={{ minWidth: 260 }}>
          <InputLabel id="eval-db-model-label">Prediction model</InputLabel>
          <Select
            labelId="eval-db-model-label"
            value={predictionModel}
            onChange={(e) => onPredictionModelChange(e.target.value as 'format' | 'unified')}
            label="Prediction model"
          >
            <MenuItem value="format">Format-specific (model for selected format)</MenuItem>
            <MenuItem value="unified">Unified (all-formats / legacy model)</MenuItem>
          </Select>
        </FormControl>
        <Button
          variant="contained"
          color="secondary"
          onClick={onEvaluateSelectedMatch}
          disabled={!canEvaluate}
        >
          {evaluating ? (
            <>
              <CircularProgress size={20} sx={{ mr: 1 }} color="inherit" />
              Evaluating…
            </>
          ) : (
            'Evaluate Selected Match'
          )}
        </Button>
      </Box>

      {/* Status and error directly below button */}
      {selectedMatchId != null && (statusMessage || error) && (
        <Box sx={{ mt: 2 }}>
          {statusMessage && (
            <Alert severity="info" sx={{ mb: error ? 1 : 0 }}>
              {statusMessage}
            </Alert>
          )}
          {error && <Alert severity="error">{error}</Alert>}
        </Box>
      )}

      {/* Progress steps while evaluating */}
      {evaluating && evaluationSteps.length > 0 && (
        <Paper
          variant="outlined"
          sx={{
            mt: 2,
            p: 2,
            pl: 2.5,
            position: 'relative',
            '&::before': {
              content: '""',
              position: 'absolute',
              left: 0,
              top: 0,
              bottom: 0,
              width: 4,
              background: accentGradient,
              borderRadius: '0 4px 4px 0',
            },
          }}
        >
          <Typography variant="subtitle2" fontWeight="bold" gutterBottom>
            Current step
          </Typography>
          <LinearProgress sx={{ mb: 2 }} />
          <List dense disablePadding>
            {evaluationSteps.map((s, idx) => (
              <ListItem key={`${s.step}-${idx}`} disablePadding sx={{ py: 0.25 }}>
                <ListItemIcon sx={{ minWidth: 32 }}>
                  {idx === evaluationSteps.length - 1 ? (
                    <CircularProgress size={16} color="primary" />
                  ) : (
                    <CheckCircleIcon color="success" fontSize="small" />
                  )}
                </ListItemIcon>
                <ListItemText primary={s.message} primaryTypographyProps={{ variant: 'body2' }} />
              </ListItem>
            ))}
          </List>
        </Paper>
      )}
    </Box>

    {/* Results */}
    {evaluationResult && (
      <Box component="section" aria-label="results-section">
        <Typography variant="h6" gutterBottom sx={{ mt: 4 }}>
          Evaluation Results
        </Typography>
        <JobModeLine
          jobUseUnifiedModel={jobUseUnifiedModel}
          jobUseLatestModel={jobUseLatestModel}
          prefix="Evaluated with "
          sx={{ mb: 1 }}
        />
        <EvaluationResults result={evaluationResult} />
      </Box>
    )}
  </Stack>
);

export default EvaluateDbSection;
