import React from 'react';
import {
  Box,
  Stack,
  Typography,
  Paper,
  Chip,
  Accordion,
  AccordionSummary,
  AccordionDetails,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ArrowForwardIcon from '@mui/icons-material/ArrowForward';
import SectionCard from './common/SectionCard';
import { MODEL_KEYS } from '../constants/defaultModelFeatures';
import type { ModelMetadataResponse } from '../types';

export interface WorkbenchModelFeaturesSectionProps {
  modelFeatures: ModelMetadataResponse;
}

const WorkbenchModelFeaturesSection: React.FC<WorkbenchModelFeaturesSectionProps> = ({
  modelFeatures,
}) => {
  return (
    <SectionCard
      title="Model features & interconnection"
      subtitle="All model types, per-format vs unified artifacts, features, outputs, and how they connect."
    >
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        The pipeline trains <strong>five core model types</strong>: <strong>batting</strong>,{' '}
        <strong>bowling</strong>, <strong>fielding</strong> (player-level), and{' '}
        <strong>extras</strong> and <strong>win</strong> (match-level). In addition, an optional{' '}
        <strong>combination meta</strong> model learns weights for team selection. All share the
        same feature families (context, form, consistency, venue, opposition, weather). Match-level
        models use aggregates of player features so team composition influences extras and win
        probability.
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
            <strong>Per-format:</strong> one model (and scaler where applicable) per format.
            Artifacts are named with the format code, e.g. <code>batting_scaler_T20.joblib</code>,{' '}
            <code>batting_model_T20.joblib</code>. Typical formats: <strong>T20</strong>,{' '}
            <strong>ODI</strong>, <strong>TEST</strong>, <strong>T20I</strong>.
          </li>
          <li>
            <strong>Unified (legacy):</strong> one model trained on all formats, e.g.{' '}
            <code>batting_scaler.joblib</code>, <code>batting_model.joblib</code>. Used when no
            per-format model is loaded or when the request does not specify a format.
          </li>
        </Box>
        <Typography variant="body2" color="text.secondary">
          At prediction time: if the request includes a format (e.g. T20) and that format&apos;s
          model is loaded, it is used; otherwise the legacy model is used. The Workbench
          &quot;Prediction model&quot; selector above lets you compare{' '}
          <strong>format-specific</strong> vs <strong>unified</strong> for accuracy trend.
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
          Optional: combination meta weights combine batting/bowling/fielding scores for team
          selection.
        </Typography>
      </Paper>

      {/* Per-model features, outputs, and artifacts */}
      <Typography variant="subtitle2" color="text.secondary" gutterBottom>
        Features, outputs, and artifacts by model
      </Typography>
      {MODEL_KEYS.map((key) => {
        const m = modelFeatures[key];
        if (!m) return null;
        return (
          <Accordion
            key={key}
            defaultExpanded={key === 'batting'}
            disableGutters
            sx={{ '&:before': { display: 'none' } }}
          >
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
                  {m.features.length} features → {m.outputs.length} output
                  {m.outputs.length !== 1 ? 's' : ''}
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
                    <strong>Per-format:</strong> {m.artifactsPattern.perFormat} — FMT = T20, ODI,
                    TEST, T20I, etc.
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
                  <Box
                    component="ul"
                    sx={{ m: 0, pl: 2, fontSize: '0.8rem', fontFamily: 'monospace' }}
                  >
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
  );
};

export default WorkbenchModelFeaturesSection;
