import React from 'react';
import { useEvaluateDb } from '../hooks/useEvaluateDb';
import { EvaluateDbSection } from './EvaluateDbSection';

const EvaluateDbTab: React.FC = () => {
  const state = useEvaluateDb();

  return (
    <EvaluateDbSection
      format={state.format}
      onFormatChange={state.setFormat}
      team1={state.team1}
      onTeam1Change={state.setTeam1}
      team2={state.team2}
      onTeam2Change={state.setTeam2}
      availableFormats={state.availableFormats}
      availableTeam1s={state.availableTeam1s}
      availableTeam2s={state.availableTeam2s}
      loading={state.loading}
      error={state.error}
      statusMessage={state.statusMessage}
      predictionModel={state.predictionModel}
      onPredictionModelChange={state.setPredictionModel}
      useLatestModel={state.useLatestModel}
      onUseLatestModelChange={state.setUseLatestModel}
      candidates={state.candidates}
      selectedMatchId={state.selectedMatchId}
      onSelectMatch={state.setSelectedMatchId}
      evaluationResult={state.evaluationResult}
      scorecard={state.scorecard}
      scorecardLoading={state.scorecardLoading}
      scorecardError={state.scorecardError}
      currentJobId={state.currentJobId}
      evaluating={state.evaluating}
      evaluationSteps={state.evaluationSteps}
      canLoad={state.canLoad}
      canEvaluate={state.canEvaluate}
      onResetOutputs={state.resetOutputs}
      onLoadCandidates={state.handleLoadCandidates}
      onEvaluateSelectedMatch={state.handleEvaluateSelectedMatch}
    />
  );
};

export default EvaluateDbTab;
