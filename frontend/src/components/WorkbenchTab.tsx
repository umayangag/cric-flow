import React from 'react';
import { Box, Typography, Alert } from '@mui/material';
import SectionCard from './common/SectionCard';
import WorkbenchRegistrySection from './WorkbenchRegistrySection';
import WorkbenchRunSection from './WorkbenchRunSection';
import { useWorkbench } from '../hooks/useWorkbench';

const WorkbenchTab: React.FC = () => {
  const {
    registryFile,
    registryError,
    registry,
    handleRegistryFile,
    runStatus,
    runStatusLoading,
    runStatusError,
  } = useWorkbench();

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
      <Alert severity="info" sx={{ mb: 1 }}>
        <Typography variant="subtitle2" gutterBottom>
          What is the Workbench?
        </Typography>
        <Typography variant="body2" component="span">
          The Workbench lets you inspect what each loaded model is: the dataset it was trained on,
          its features, and its artifacts. How well those models predict is the{' '}
          <strong>Evaluation report</strong> tab, which reads the harness&apos;s own measurements
          rather than re-scoring matches here.
        </Typography>
        <Typography variant="body2" component="div" sx={{ mt: 1 }}>
          Running the pipeline lives in <strong>Ops → Pipeline</strong>, which shows each
          step&apos;s real state. What each step does is in <code>docs/overview.md</code> and{' '}
          <code>docs/ml-and-training.md</code>.
        </Typography>
      </Alert>

      <WorkbenchRegistrySection
        registryFile={registryFile}
        registryError={registryError}
        registry={registry}
        onFileChange={handleRegistryFile}
      />

      <WorkbenchRunSection status={runStatus} loading={runStatusLoading} error={runStatusError} />

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
