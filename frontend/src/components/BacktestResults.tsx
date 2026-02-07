import React from 'react';
import type { BacktestEvaluateResponse, BacktestPlayerResult } from '../api/types';

export type BacktestResultsProps = {
  result: BacktestEvaluateResponse;
};

function num(v: unknown): string {
  return typeof v === 'number' ? v.toString() : '';
}

export const BacktestResults: React.FC<BacktestResultsProps> = ({ result }) => {
  const players: BacktestPlayerResult[] = Array.isArray(result.players) ? result.players : [];
  const ma = result.match_aggregates as any;
  const metrics = (result.metrics || {}) as Record<string, number>;
  const modelVersion = (result.filters as any)?.model_version as string | undefined;

  return (
    <div>
      <section aria-label="metrics-summary" style={{ marginBottom: 16 }}>
        <h4>Metrics</h4>
        <div>player_runs_mae: {num(metrics['player_runs_mae'])}</div>
        {'player_runs_rmse' in metrics && (
          <div>player_runs_rmse: {num(metrics['player_runs_rmse'])}</div>
        )}
        {'winner_accuracy' in metrics && (
          <div>winner_accuracy: {num(metrics['winner_accuracy'])}</div>
        )}
        {modelVersion && <div>model_version: {modelVersion}</div>}
      </section>

      {ma && (
        <section aria-label="match-aggregates" style={{ marginBottom: 16 }}>
          <h4>Match aggregates</h4>
          <div style={{ display: 'flex', gap: 24 }}>
            <div>
              <div><strong>Predicted</strong></div>
              <div>runs: {num(ma.predicted?.runs)}</div>
              <div>wickets: {num(ma.predicted?.wickets)}</div>
              <div>extras: {num(ma.predicted?.extras)}</div>
              <div>winner_team_code: {String(ma.predicted?.winner_team_code ?? '')}</div>
            </div>
            <div>
              <div><strong>Actual</strong></div>
              <div>runs: {num(ma.actual?.runs)}</div>
              <div>wickets: {num(ma.actual?.wickets)}</div>
              <div>extras: {num(ma.actual?.extras)}</div>
              <div>winner_team_code: {String(ma.actual?.winner_team_code ?? '')}</div>
            </div>
            <div>
              <div><strong>Errors (MAE)</strong></div>
              <div>runs_mae: {num(ma.errors?.runs_mae)}</div>
              <div>wickets_mae: {num(ma.errors?.wickets_mae)}</div>
              <div>extras_mae: {num(ma.errors?.extras_mae)}</div>
            </div>
          </div>
        </section>
      )}

      <section>
        <h4>Players</h4>
        <table aria-label="players-table" style={{ borderCollapse: 'collapse', width: '100%' }}>
          <thead>
            <tr>
              <th style={{ textAlign: 'left' }}>player_id</th>
              <th style={{ textAlign: 'left' }}>pred.runs</th>
              <th style={{ textAlign: 'left' }}>act.runs</th>
              <th style={{ textAlign: 'left' }}>err.runs_mae</th>
            </tr>
          </thead>
          <tbody>
            {players.map((p) => (
              <tr key={p.player_id}>
                <td>{p.player_id}</td>
                <td>{num(p.predicted?.['runs'])}</td>
                <td>{num(p.actual?.['runs'])}</td>
                <td>{num(p.errors?.['runs_mae'])}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </div>
  );
};

export default BacktestResults;
