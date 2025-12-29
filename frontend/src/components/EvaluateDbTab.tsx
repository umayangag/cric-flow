import React, { useMemo, useState } from 'react';
import { api } from '../api';
import type { BacktestCandidate, BacktestEvaluateResponse } from '../types';

const formats = ['TEST', 'ODI', 'T20I', 'T20'] as const;

const EvaluateDbTab: React.FC = () => {
  // Inputs for new backtest flow
  const [format, setFormat] = useState<string>('T20');
  const [team1, setTeam1] = useState<string>('IND');
  const [team2, setTeam2] = useState<string>('AUS');

  // UI state
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<string>('');

  // Backtest data
  const [candidates, setCandidates] = useState<BacktestCandidate[]>([]);
  const [selectedMatchId, setSelectedMatchId] = useState<number | null>(null);
  const [result, setResult] = useState<BacktestEvaluateResponse | null>(null);

  const canLoad = useMemo(() => !!format && !!team1 && !!team2 && !loading, [format, team1, team2, loading]);
  const canEvaluate = useMemo(
    () => !!format && !!team1 && !!team2 && selectedMatchId != null && !loading,
    [format, team1, team2, selectedMatchId, loading]
  );

  const onResetOutputs = () => {
    setCandidates([]);
    setSelectedMatchId(null);
    setResult(null);
    setStatus('');
    setError(null);
  };

  const onLoadCandidates = async () => {
    try {
      setLoading(true);
      setError(null);
      setResult(null);
      setStatus('Loading played matches…');
      const resp = await api.backtestSelect(format, team1.trim(), team2.trim());
      setCandidates(resp.candidates || []);
      setStatus(`Loaded ${resp.candidates?.length ?? 0} candidates for ${team1} vs ${team2} (${format}).`);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
      setStatus('');
    } finally {
      setLoading(false);
    }
  };

  const onEvaluate = async () => {
    if (selectedMatchId == null) return;
    try {
      setLoading(true);
      setError(null);
      setStatus('Evaluating…');
      const resp = await api.backtestEvaluate(format, team1.trim(), team2.trim(), selectedMatchId);
      setResult(resp);
      setStatus('Evaluation complete.');
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
      setStatus('');
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
          <select value={format} onChange={(e) => { setFormat(e.target.value); onResetOutputs(); }}>
            {formats.map((f) => (
              <option key={f} value={f}>{f}</option>
            ))}
          </select>
        </label>
        <label>
          Team 1:&nbsp;
          <input value={team1} onChange={(e) => { setTeam1(e.target.value); onResetOutputs(); }} placeholder="e.g., IND" />
        </label>
        <label>
          Team 2:&nbsp;
          <input value={team2} onChange={(e) => { setTeam2(e.target.value); onResetOutputs(); }} placeholder="e.g., AUS" />
        </label>
        <button onClick={onLoadCandidates} disabled={!canLoad}>Load Played Matches</button>
      </div>

      {status && (
        <div style={{ marginBottom: 12 }} aria-live="polite" aria-atomic="true">{status}</div>
      )}
      {error && <div style={{ marginBottom: 12, color: 'red' }}>Error: {error}</div>}

      {/* Candidates */}
      <section aria-label="candidates-section" style={{ marginBottom: 16 }}>
        <h3 style={{ margin: '8px 0' }}>Candidates</h3>
        {candidates.length === 0 ? (
          <div style={{ color: '#666' }}>No candidates loaded yet.</div>
        ) : (
          <div style={{ maxHeight: 240, overflow: 'auto', border: '1px solid #eee', padding: 8 }}>
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr>
                  <th style={{ textAlign: 'left', borderBottom: '1px solid #ddd', padding: 6 }}>Date</th>
                  <th style={{ textAlign: 'left', borderBottom: '1px solid #ddd', padding: 6 }}>Match</th>
                  <th style={{ textAlign: 'left', borderBottom: '1px solid #ddd', padding: 6 }}>Venue</th>
                  <th style={{ textAlign: 'left', borderBottom: '1px solid #ddd', padding: 6 }}>Winner</th>
                  <th style={{ textAlign: 'left', borderBottom: '1px solid #ddd', padding: 6 }}>Action</th>
                </tr>
              </thead>
              <tbody>
                {candidates.map((c) => (
                  <tr key={c.match_id}>
                    <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6 }}>{new Date(c.date).toISOString().slice(0, 10)}</td>
                    <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6 }}>{c.team1} vs {c.team2}</td>
                    <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6 }}>{c.venue || '-'}</td>
                    <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6 }}>{c.winner_team_code || '-'}</td>
                    <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6 }}>
                      <label>
                        <input
                          type="radio"
                          name="selectedMatch"
                          checked={selectedMatchId === c.match_id}
                          onChange={() => setSelectedMatchId(c.match_id)}
                        />
                        &nbsp;Select
                      </label>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <div style={{ marginTop: 8 }}>
          <button onClick={onEvaluate} disabled={!canEvaluate}>{loading ? 'Evaluating…' : 'Evaluate Selected Match'}</button>
        </div>
      </section>

      {/* Results */}
      <section aria-label="results-section">
        <h3 style={{ margin: '8px 0' }}>Results</h3>
        {!result ? (
          <div style={{ color: '#666' }}>Run an evaluation to see player-level errors and summary metrics.</div>
        ) : (
          <div>
            <div style={{ marginBottom: 8 }}>
              Match: <strong>{result.match.match_id}</strong> · Date: <strong>{new Date(result.match.date).toISOString().slice(0,10)}</strong>
            </div>
            <div style={{ marginBottom: 12 }}>
              Metrics: player_runs_mae = <strong>{result.metrics?.player_runs_mae?.toFixed(3)}</strong>
              {typeof result.metrics?.player_runs_rmse === 'number' && (
                <>
                  {` · player_runs_rmse = `}
                  <strong>{result.metrics.player_runs_rmse.toFixed(3)}</strong>
                </>
              )}
              {typeof result.metrics?.player_runs_r2 === 'number' && (
                <>
                  {` · player_runs_r2 = `}
                  <strong>{result.metrics.player_runs_r2.toFixed(3)}</strong>
                </>
              )}
              {typeof result.metrics?.player_wickets_mae === 'number' && (
                <>
                  {` · player_wickets_mae = `}
                  <strong>{result.metrics.player_wickets_mae.toFixed(3)}</strong>
                </>
              )}
              {typeof result.metrics?.player_economy_mae === 'number' && (
                <>
                  {` · player_economy_mae = `}
                  <strong>{result.metrics.player_economy_mae.toFixed(3)}</strong>
                </>
              )}
              {typeof result.metrics?.player_catches_mae === 'number' && (
                <>
                  {` · player_catches_mae = `}
                  <strong>{result.metrics.player_catches_mae.toFixed(3)}</strong>
                </>
              )}
              {typeof result.metrics?.player_run_outs_mae === 'number' && (
                <>
                  {` · player_run_outs_mae = `}
                  <strong>{result.metrics.player_run_outs_mae.toFixed(3)}</strong>
                </>
              )}
              {typeof result.metrics?.match_runs_mae === 'number' && (
                <>
                  {` · match_runs_mae = `}
                  <strong>{result.metrics.match_runs_mae.toFixed(3)}</strong>
                </>
              )}
              {typeof result.metrics?.match_wickets_mae === 'number' && (
                <>
                  {` · match_wickets_mae = `}
                  <strong>{result.metrics.match_wickets_mae.toFixed(3)}</strong>
                </>
              )}
              {typeof result.metrics?.match_extras_mae === 'number' && (
                <>
                  {` · match_extras_mae = `}
                  <strong>{result.metrics.match_extras_mae.toFixed(3)}</strong>
                </>
              )}
              {typeof result.metrics?.winner_accuracy === 'number' && (
                <>
                  {` · winner_accuracy = `}
                  <strong>{result.metrics.winner_accuracy.toFixed(0)}</strong>
                </>
              )}
            </div>
            {result.match_aggregates && (
              <div style={{ marginBottom: 12, border: '1px solid #eee', padding: 8 }}>
                <div style={{ fontWeight: 600, marginBottom: 6 }}>Match aggregates</div>
                <div style={{ display: 'flex', gap: 24, flexWrap: 'wrap' }}>
                  <div>
                    <div style={{ fontWeight: 600 }}>Predicted</div>
                    <div>runs: <strong>{String(result.match_aggregates.predicted?.runs ?? '-')}</strong></div>
                    <div>wickets: <strong>{String(result.match_aggregates.predicted?.wickets ?? '-')}</strong></div>
                    <div>extras: <strong>{String(result.match_aggregates.predicted?.extras ?? '-')}</strong></div>
                    <div>winner: <strong>{String(result.match_aggregates.predicted?.winner_team_code ?? '-')}</strong></div>
                  </div>
                  <div>
                    <div style={{ fontWeight: 600 }}>Actual</div>
                    <div>runs: <strong>{String(result.match_aggregates.actual?.runs ?? '-')}</strong></div>
                    <div>wickets: <strong>{String(result.match_aggregates.actual?.wickets ?? '-')}</strong></div>
                    <div>extras: <strong>{String(result.match_aggregates.actual?.extras ?? '-')}</strong></div>
                    <div>winner: <strong>{String(result.match_aggregates.actual?.winner_team_code ?? '-')}</strong></div>
                  </div>
                  <div>
                    <div style={{ fontWeight: 600 }}>Errors</div>
                    <div>runs_mae: <strong>{String(result.match_aggregates.errors?.runs_mae ?? '-')}</strong></div>
                    <div>wickets_mae: <strong>{String(result.match_aggregates.errors?.wickets_mae ?? '-')}</strong></div>
                    <div>extras_mae: <strong>{String(result.match_aggregates.errors?.extras_mae ?? '-')}</strong></div>
                  </div>
                </div>
              </div>
            )}
            <div style={{ maxHeight: 320, overflow: 'auto', border: '1px solid #eee', padding: 8 }}>
              {(() => {
                // Determine if bowling metrics are present in any row
                const anyWickets = result.players?.some((p) => typeof p.predicted?.wickets === 'number' || typeof p.actual?.wickets === 'number');
                const anyEconomy = result.players?.some((p) => typeof p.predicted?.economy === 'number' || typeof p.actual?.economy === 'number');
                const anyCatches = result.players?.some((p) => typeof (p.predicted as any)?.catches === 'number' || typeof (p.actual as any)?.catches === 'number');
                const anyRunOuts = result.players?.some((p) => typeof (p.predicted as any)?.run_outs === 'number' || typeof (p.actual as any)?.run_outs === 'number');
                return (
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr>
                    <th style={{ textAlign: 'left', borderBottom: '1px solid #ddd', padding: 6 }}>Player ID</th>
                    <th style={{ textAlign: 'right', borderBottom: '1px solid #ddd', padding: 6 }}>Pred Runs</th>
                    <th style={{ textAlign: 'right', borderBottom: '1px solid #ddd', padding: 6 }}>Actual Runs</th>
                    <th style={{ textAlign: 'right', borderBottom: '1px solid #ddd', padding: 6 }}>Abs Error</th>
                    {anyWickets && (
                      <>
                        <th style={{ textAlign: 'right', borderBottom: '1px solid #ddd', padding: 6 }}>Pred Wkts</th>
                        <th style={{ textAlign: 'right', borderBottom: '1px solid #ddd', padding: 6 }}>Actual Wkts</th>
                        <th style={{ textAlign: 'right', borderBottom: '1px solid #ddd', padding: 6 }}>Wkts Abs Err</th>
                      </>
                    )}
                    {anyEconomy && (
                      <>
                        <th style={{ textAlign: 'right', borderBottom: '1px solid #ddd', padding: 6 }}>Pred Econ</th>
                        <th style={{ textAlign: 'right', borderBottom: '1px solid #ddd', padding: 6 }}>Actual Econ</th>
                        <th style={{ textAlign: 'right', borderBottom: '1px solid #ddd', padding: 6 }}>Econ Abs Err</th>
                      </>
                    )}
                    {anyCatches && (
                      <>
                        <th style={{ textAlign: 'right', borderBottom: '1px solid #ddd', padding: 6 }}>Pred Catches</th>
                        <th style={{ textAlign: 'right', borderBottom: '1px solid #ddd', padding: 6 }}>Actual Catches</th>
                        <th style={{ textAlign: 'right', borderBottom: '1px solid #ddd', padding: 6 }}>Catches Abs Err</th>
                      </>
                    )}
                    {anyRunOuts && (
                      <>
                        <th style={{ textAlign: 'right', borderBottom: '1px solid #ddd', padding: 6 }}>Pred Run Outs</th>
                        <th style={{ textAlign: 'right', borderBottom: '1px solid #ddd', padding: 6 }}>Actual Run Outs</th>
                        <th style={{ textAlign: 'right', borderBottom: '1px solid #ddd', padding: 6 }}>Run Outs Abs Err</th>
                      </>
                    )}
                  </tr>
                </thead>
                <tbody>
                  {result.players.map((p) => (
                    <tr key={p.player_id}>
                      <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6 }}>{p.player_id}</td>
                      <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{p.predicted?.runs?.toFixed?.(2) ?? p.predicted?.runs}</td>
                      <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{p.actual?.runs?.toFixed?.(2) ?? p.actual?.runs}</td>
                      <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{p.errors?.runs_mae?.toFixed?.(2) ?? p.errors?.runs_mae}</td>
                      {anyWickets && (
                        <>
                          <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{p.predicted?.wickets?.toFixed?.(2) ?? (p.predicted?.wickets as any)}</td>
                          <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{p.actual?.wickets?.toFixed?.(2) ?? (p.actual?.wickets as any)}</td>
                          <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{p.errors?.wickets_mae?.toFixed?.(2) ?? (p.errors?.wickets_mae as any)}</td>
                        </>
                      )}
                      {anyEconomy && (
                        <>
                          <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{p.predicted?.economy?.toFixed?.(2) ?? (p.predicted?.economy as any)}</td>
                          <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{p.actual?.economy?.toFixed?.(2) ?? (p.actual?.economy as any)}</td>
                          <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{p.errors?.economy_mae?.toFixed?.(2) ?? (p.errors?.economy_mae as any)}</td>
                        </>
                      )}
                      {anyCatches && (
                        <>
                          <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{(p.predicted as any)?.catches ?? ''}</td>
                          <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{(p.actual as any)?.catches ?? ''}</td>
                          <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{(p.errors as any)?.catches_mae ?? ''}</td>
                        </>
                      )}
                      {anyRunOuts && (
                        <>
                          <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{(p.predicted as any)?.run_outs ?? ''}</td>
                          <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{(p.actual as any)?.run_outs ?? ''}</td>
                          <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{(p.errors as any)?.run_outs_mae ?? ''}</td>
                        </>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
                );
              })()}
            </div>
          </div>
        )}
      </section>
    </div>
  );
};

export default EvaluateDbTab;
