import React from 'react';
import { Box, Typography, Alert } from '@mui/material';
import SectionCard from './common/SectionCard';
import WorkbenchAccuracyTrendSection from './WorkbenchAccuracyTrendSection';
import WorkbenchPipelineInfoSection from './WorkbenchPipelineInfoSection';
import WorkbenchRegistrySection from './WorkbenchRegistrySection';
import WorkbenchModelFeaturesSection from './WorkbenchModelFeaturesSection';
import { useWorkbench } from '../hooks/useWorkbench';
import { getTrainableModelKeys, hasCombinationMeta, getModelModes } from '../utils/modelMetadata';

const WorkbenchTab: React.FC = () => {
  const {
    format,
    setFormat,
    predictionModel,
    setPredictionModel,
    startDate,
    setStartDate,
    endDate,
    setEndDate,
    limit,
    setLimit,
    maxLimit,
    availableFormats,
    trendLoading,
    trendError,
    trendData,
    loadAccuracyTrend,
    registryFile,
    registryError,
    registry,
    handleRegistryFile,
    modelMetadata,
    modelMetadataLoading,
    modelMetadataError,
  } = useWorkbench();

  const trainableModelKeys = getTrainableModelKeys(modelMetadata);
  const combinationMeta = hasCombinationMeta(modelMetadata);
  const modelModes = getModelModes(modelMetadata);

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

      <WorkbenchPipelineInfoSection
        trainableModelKeys={trainableModelKeys.length > 0 ? trainableModelKeys : undefined}
        hasCombinationMeta={combinationMeta}
      />

      <WorkbenchAccuracyTrendSection
        format={format}
        availableFormats={availableFormats}
        predictionModel={predictionModel}
        startDate={startDate}
        endDate={endDate}
        limit={limit}
        maxLimit={maxLimit}
        trendLoading={trendLoading}
        trendError={trendError}
        trendData={trendData}
        onChangeFormat={setFormat}
        onChangePredictionModel={(value) => setPredictionModel(value)}
        onChangeStartDate={setStartDate}
        onChangeEndDate={setEndDate}
        onChangeLimit={(value) => setLimit(value)}
        onLoad={loadAccuracyTrend}
        modelModes={modelModes.length > 0 ? modelModes : undefined}
      />

      <WorkbenchRegistrySection
        registryFile={registryFile}
        registryError={registryError}
        registry={registry}
        onFileChange={handleRegistryFile}
      />

      <WorkbenchModelFeaturesSection
        modelMetadata={modelMetadata}
        loading={modelMetadataLoading}
        error={modelMetadataError}
      />

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
