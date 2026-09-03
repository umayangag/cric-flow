import React from 'react';
import { Box, Stack, Typography } from '@mui/material';
import { metricSpectrumGradient } from '../../utils/metricColor';

/**
 * What the colours on this page mean, said once.
 *
 * A red-to-green number is a claim, and a reader is owed the two things behind it: that
 * the ends are the metric's own reference band and not a page-wide guess, and that an
 * uncoloured number is uncoloured on purpose.
 */
const MetricSpectrumLegend: React.FC = () => (
  <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" sx={{ mb: 2 }}>
    <Typography variant="caption" color="text.secondary">
      worse
    </Typography>
    <Box
      aria-hidden
      sx={{
        width: 120,
        height: 8,
        borderRadius: 4,
        background: metricSpectrumGradient,
      }}
    />
    <Typography variant="caption" color="text.secondary">
      better
    </Typography>
    <Typography variant="caption" color="text.secondary">
      — each number is painted against its own reference band, from the glossary behind the ⓘ beside
      its label. A metric with no band of its own — an interval width, which is progress only while
      its coverage holds — is left uncoloured.
    </Typography>
  </Stack>
);

export default MetricSpectrumLegend;
