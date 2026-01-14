import React, { useMemo, useState } from 'react';
import { api } from '../api';
import type { BacktestCandidate, BacktestEvaluateResponse } from '../types';
import CandidatesTable from './CandidatesTable';
import EvaluationResults from './EvaluationResults';

const formats = ['TEST', 'ODI', 'T20I', 'T20'] as const;

const EvaluateDbTab: React.FC = () => {
  // Inputs for new backtest flow
  const [format, setFormat] = useState<string>('T20');
  const [team1, setTeam1] = useState<string>('IND');
  const [team2, setTeam2] = useState<string>('AUS');

  // UI state
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [statusMessage, setStatusMessage] = useState<string>('');

  // Backtest data
  const [candidates, setCandidates] = useState<BacktestCandidate[]>([]);
  const [selectedMatchId, setSelectedMatchId] = useState<number | null>(null);
  const [evaluationResult, setEvaluationResult] = useState<BacktestEvaluateResponse | null>(null);

  const canLoad = useMemo(() => !!format && !!team1 && !!team2 && !loading, [format, team1, team2, loading]);
  const canEvaluate = useMemo(
    () => !!format && !!team1 && !!team2 && selectedMatchId != null && !loading,
    [format, team1, team2, selectedMatchId, loading]
  );

  const resetOutputs = () => {
    setCandidates([]);
    setSelectedMatchId(null);
    setEvaluationResult(null);
    setStatusMessage('');
    setError(null);
  };

  const handleLoadCandidates = async () => {
    try {
      setLoading(true);
      setError(null);
      setEvaluationResult(null);
      setStatusMessage('Loading played matches…');
      const resp = await api.backtestSelect(format, team1.trim(), team2.trim());
      setCandidates(resp.candidates || []);
      setStatusMessage(`Loaded ${resp.candidates?.length ?? 0} candidates for ${team1} vs ${team2} (${format}).`);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
      setStatusMessage('');
    } finally {
      setLoading(false);
    }
  };

  const handleEvaluateSelectedMatch = async () => {
    if (selectedMatchId == null) return;
    try {
      setLoading(true);
      setError(null);
      setStatusMessage('Evaluating…');
      const resp = await api.backtestEvaluate(format, team1.trim(), team2.trim(), selectedMatchId);
      setEvaluationResult(resp);
      setStatusMessage('Evaluation complete.');
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
      setStatusMessage('');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <p style={{ marginTop: 0 }}>
        Evaluate historical matches by training strictly up to the match date, predicting for actual players, and
        comparing predictions vs actuals.
      </p>

      <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap', marginBottom: 12 }}>
        <label>
          Format:&nbsp;
          <select value={format} onChange={(e) => { setFormat(e.target.value); resetOutputs(); }}>
            {formats.map((f) => (
              <option key={f} value={f}>{f}</option>
            ))}
          </select>
        </label>
        <label>
          Team 1:&nbsp;
          <input value={team1} onChange={(e) => { setTeam1(e.target.value); resetOutputs(); }} placeholder="e.g., IND" />
        </label>
        <label>
          Team 2:&nbsp;
          <input value={team2} onChange={(e) => { setTeam2(e.target.value); resetOutputs(); }} placeholder="e.g., AUS" />
        </label>
        <button onClick={handleLoadCandidates} disabled={!canLoad}>Load Played Matches</button>
      </div>

      {statusMessage && (
        <div style={{ marginBottom: 12 }} aria-live="polite" aria-atomic="true">{statusMessage}</div>
      )}
      {error && <div style={{ marginBottom: 12, color: 'red' }}>Error: {error}</div>}

      {/* Candidates */}
      <section aria-label="candidates-section" style={{ marginBottom: 16 }}>
        <h3 style={{ margin: '8px 0' }}>Candidates</h3>
        <CandidatesTable
          candidates={candidates}
          selectedMatchId={selectedMatchId}
          onSelectMatch={setSelectedMatchId}
        />
        <div style={{ marginTop: 8 }}>
          <button onClick={handleEvaluateSelectedMatch} disabled={!canEvaluate}>{loading ? 'Evaluating…' : 'Evaluate Selected Match'}</button>
        </div>
      </section>

      {/* Results */}
      <section aria-label="results-section">
        <h3 style={{ margin: '8px 0' }}>Results</h3>
        {!evaluationResult ? (
          <div style={{ color: '#666' }}>Run an evaluation to see player-level errors and summary metrics.</div>
        ) : (
          <EvaluationResults result={evaluationResult} />
        )}
      </section>
    </div>
  );
};

export default EvaluateDbTab;
