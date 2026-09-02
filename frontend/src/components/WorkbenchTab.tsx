import React from 'react';
import { Box, Typography, Alert } from '@mui/material';
import SectionCard from './common/SectionCard';
import WorkbenchRunSection from './WorkbenchRunSection';
import { useWorkbench } from '../hooks/useWorkbench';

const WorkbenchTab: React.FC = () => {
  const { runStatus, runStatusLoading, runStatusError } = useWorkbench();

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
              <strong>Build a run</strong> — <code>make retrain CUTOFF=2025-09-01</code>, then{' '}
              <code>make reload</code> to serve it. The run id, cutoff, dataset digest, commit and
              chosen hyperparameters shown above come from that run&apos;s{' '}
              <code>manifest.json</code>.
            </li>
            <li>
              <strong>Score it</strong> — <code>make evaluate</code> runs L4&apos;s harness and
              writes the report the <strong>Evaluation report</strong> tab renders. It touches no
              artifact <code>current</code> points at.
            </li>
            <li>
              <strong>Walk-forward numbers</strong> — per-fold, per-format tables are part of that
              report: the <strong>Evaluation report</strong> tab renders them. Nothing is uploaded
              here; the harness is the only thing that measures folds.
            </li>
          </Box>
        </Typography>
      </SectionCard>
    </Box>
  );
};

export default WorkbenchTab;
