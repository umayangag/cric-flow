import React, { useState } from 'react';
import { Box, IconButton, Popover, Stack, Typography } from '@mui/material';
import { InfoOutlined as InfoOutlinedIcon } from '@mui/icons-material';
import { useMetricGlossary } from '../../context/MetricGlossaryContext';

type MetricInfoProps = {
  /** The key the service reports this number under, e.g. `objective_auc`. */
  metricKey: string;
  /** The number as this surface shows it, placed on the reference band in the popover. */
  value?: string;
  /** What the label says, for the button's accessible name only — never shown as prose. */
  label?: string;
};

/**
 * L-1: the explainer behind every metric the app shows.
 *
 * One component, on every metric label, resolving its key against the glossary the
 * service serves — so `dispersion ratio 1.02` can be read by someone who has not lived
 * inside the plan, and so the sentence that explains it is written once, beside the code
 * that computes it. A key the glossary does not carry renders nothing at all: an
 * explainer that invented its own copy would be the duplication this exists to remove.
 */
export const MetricInfo: React.FC<MetricInfoProps> = ({ metricKey, value, label }) => {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const entry = useMetricGlossary()(metricKey);
  if (!entry) return null;

  return (
    <>
      <IconButton
        size="small"
        aria-label={`What is ${label ?? entry.name}?`}
        onClick={(event) => setAnchor(event.currentTarget)}
        sx={{ p: 0.25, ml: 0.5, verticalAlign: 'middle', color: 'text.disabled' }}
      >
        <InfoOutlinedIcon sx={{ fontSize: 15 }} />
      </IconButton>
      <Popover
        open={!!anchor}
        anchorEl={anchor}
        onClose={() => setAnchor(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        slotProps={{ paper: { sx: { p: 2, maxWidth: 380 } } }}
      >
        <Stack spacing={1}>
          <Typography variant="subtitle2" fontWeight={600}>
            {entry.name}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {entry.explanation}
          </Typography>
          {value && (
            <Typography variant="body2">
              <strong>Here:</strong> {value}
            </Typography>
          )}
          <Box>
            <Typography variant="caption" color="text.secondary" component="div">
              <strong>Reference:</strong> {entry.band}
            </Typography>
            <Typography variant="caption" color="text.secondary" component="div">
              <strong>Direction:</strong> {entry.better}
            </Typography>
          </Box>
        </Stack>
      </Popover>
    </>
  );
};

type MetricLabelProps = MetricInfoProps & {
  /** The visible label. Column headings and tile captions keep the wording they had. */
  label: string;
};

/** A label with its explainer beside it: what every metric-bearing heading renders. */
export const MetricLabel: React.FC<MetricLabelProps> = ({ metricKey, label, value }) => (
  <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center' }}>
    {label}
    <MetricInfo metricKey={metricKey} label={label} value={value} />
  </Box>
);

export default MetricLabel;
