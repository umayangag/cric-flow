import React from 'react';
import { Box } from '@mui/material';
import Typography from '@mui/material/Typography';
import Checkbox from '@mui/material/Checkbox';
import FormControl from '@mui/material/FormControl';
import FormControlLabel from '@mui/material/FormControlLabel';
import InputLabel from '@mui/material/InputLabel';
import MenuItem from '@mui/material/MenuItem';
import Select from '@mui/material/Select';
import TextField from '@mui/material/TextField';
import { useCanonicalFormats } from '../hooks/useCanonicalFormats';

const AUTO_TUNE_MODELS = [
  { value: 'all', label: 'All' },
  { value: 'batting', label: 'Batting' },
  { value: 'bowling', label: 'Bowling' },
  { value: 'fielding', label: 'Fielding' },
  { value: 'extras', label: 'Extras' },
  { value: 'win', label: 'Win' },
  { value: 'innings', label: 'Innings' },
] as const;

export const AUTO_TUNE_ALGORITHMS = [
  { value: 'rf', label: 'Random Forest' },
  { value: 'gb', label: 'Gradient Boosting' },
  { value: 'quantile', label: 'Quantile Regressor' },
  { value: 'et', label: 'Extra Trees' },
  { value: 'hgb', label: 'Hist Gradient Boosting' },
  { value: 'stacked', label: 'Stacking Regressor' },
  { value: 'mlp', label: 'MLP (Neural Network)' },
] as const;

export interface AutoTuneFormProps {
  model: string;
  onModelChange: (v: string) => void;
  format: string;
  onFormatChange: (v: string) => void;
  rescreen: boolean;
  onRescreenChange: (v: boolean) => void;
  cutoff: string;
  onCutoffChange: (v: string) => void;
  algorithms: Set<string>;
  onAlgorithmsChange: (v: Set<string>) => void;
}

const AutoTuneForm: React.FC<AutoTuneFormProps> = ({
  model,
  onModelChange,
  format,
  onFormatChange,
  rescreen,
  onRescreenChange,
  cutoff,
  onCutoffChange,
  algorithms,
  onAlgorithmsChange,
}) => {
  const { formats: canonicalFormats } = useCanonicalFormats();
  const formatOptions = [
    { value: 'unified', label: 'Unified only' },
    { value: '', label: 'All formats' },
    ...canonicalFormats.map((f) => ({ value: f, label: f })),
  ];

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, mb: 1.5 }}>
      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 2 }}>
        <FormControl size="small" sx={{ minWidth: 140 }}>
          <InputLabel id="autotune-model-label">Model</InputLabel>
          <Select
            labelId="autotune-model-label"
            value={model}
            label="Model"
            onChange={(e) => onModelChange(e.target.value)}
          >
            {AUTO_TUNE_MODELS.map((o) => (
              <MenuItem key={o.value} value={o.value}>
                {o.label}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <FormControl size="small" sx={{ minWidth: 140 }}>
          <InputLabel id="autotune-format-label">Format</InputLabel>
          <Select
            labelId="autotune-format-label"
            value={format}
            label="Format"
            onChange={(e) => onFormatChange(e.target.value)}
          >
            {formatOptions.map((o) => (
              <MenuItem key={o.value || 'all'} value={o.value}>
                {o.label}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
      </Box>
      <FormControlLabel
        control={
          <Checkbox
            checked={rescreen}
            onChange={(e) => onRescreenChange(e.target.checked)}
            size="small"
          />
        }
        label="Rescreen: start from scratch (full algorithm search, ignore prior tuning)"
      />
      <TextField
        size="small"
        label="Cutoff (optional, RFC3339)"
        placeholder="2024-01-01T00:00:00Z"
        value={cutoff}
        onChange={(e) => onCutoffChange(e.target.value)}
        sx={{ maxWidth: 320 }}
        helperText="Training data cutoff for API. Leave empty to use default."
      />
      <Box>
        <Typography variant="caption" color="text.secondary" display="block" sx={{ mb: 0.5 }}>
          Algorithms to consider (only selected will be used). Default: last used for this
          model+format.
        </Typography>
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
          {AUTO_TUNE_ALGORITHMS.map(({ value, label }) => (
            <FormControlLabel
              key={value}
              control={
                <Checkbox
                  checked={algorithms.has(value)}
                  onChange={(e) => {
                    const next = new Set(algorithms);
                    if (e.target.checked) next.add(value);
                    else next.delete(value);
                    onAlgorithmsChange(next);
                  }}
                  size="small"
                />
              }
              label={label}
            />
          ))}
        </Box>
      </Box>
    </Box>
  );
};

export default AutoTuneForm;
