import React, { useMemo } from 'react';
import type { BacktestEvaluateResponse } from '../types';

type EvaluationResultsProps = {
  result: BacktestEvaluateResponse;
};

const MetricsLine: React.FC<{ metrics: Record<string, number> | undefined }> = ({ metrics }) => {
  const parts = useMemo(() => {
    if (!metrics) return [] as string[];
    const order = [
      'player_runs_mae', 'player_runs_rmse', 'player_runs_r2',
      'player_wickets_mae', 'player_economy_mae', 'player_catches_mae', 'player_run_outs_mae',
      'match_runs_mae', 'match_wickets_mae', 'match_extras_mae', 'winner_accuracy',
    ];
    return order
      .filter((k) => typeof metrics[k] === 'number')
      .map((k) => `${k} = ${formatMetric(metrics[k])}`);
  }, [metrics]);

  if (!parts.length) return null;
  return (
    <div style={{ marginBottom: 12 }}>
      Metrics: {parts.map((p, i) => (
        <React.Fragment key={i}>
          <strong>{p}</strong>
          {i < parts.length - 1 ? ' · ' : ''}
        </React.Fragment>
      ))}
    </div>
  );
};

function formatMetric(v: number | undefined): string {
  if (typeof v !== 'number') return '-';
  const isIntLike = Math.abs(v - Math.round(v)) < 1e-9;
  return isIntLike ? String(Math.round(v)) : v.toFixed(3);
}

const MatchAggregates: React.FC<{
  aggregates: BacktestEvaluateResponse['match_aggregates'];
}> = ({ aggregates }) => {
  if (!aggregates) return null;
  return (
    <div style={{ marginBottom: 12, border: '1px solid #eee', padding: 8 }}>
      <div style={{ fontWeight: 600, marginBottom: 6 }}>Match aggregates</div>
      <div style={{ display: 'flex', gap: 24, flexWrap: 'wrap' }}>
        <div>
          <div style={{ fontWeight: 600 }}>Predicted</div>
          <div>runs: <strong>{String(aggregates.predicted?.runs ?? '-')}</strong></div>
          <div>wickets: <strong>{String(aggregates.predicted?.wickets ?? '-')}</strong></div>
          <div>extras: <strong>{String(aggregates.predicted?.extras ?? '-')}</strong></div>
          <div>winner: <strong>{String(aggregates.predicted?.winner_team_code ?? '-')}</strong></div>
        </div>
        <div>
          <div style={{ fontWeight: 600 }}>Actual</div>
          <div>runs: <strong>{String(aggregates.actual?.runs ?? '-')}</strong></div>
          <div>wickets: <strong>{String(aggregates.actual?.wickets ?? '-')}</strong></div>
          <div>extras: <strong>{String(aggregates.actual?.extras ?? '-')}</strong></div>
          <div>winner: <strong>{String(aggregates.actual?.winner_team_code ?? '-')}</strong></div>
        </div>
        <div>
          <div style={{ fontWeight: 600 }}>Errors</div>
          <div>runs_mae: <strong>{String(aggregates.errors?.runs_mae ?? '-')}</strong></div>
          <div>wickets_mae: <strong>{String(aggregates.errors?.wickets_mae ?? '-')}</strong></div>
          <div>extras_mae: <strong>{String(aggregates.errors?.extras_mae ?? '-')}</strong></div>
        </div>
      </div>
    </div>
  );
};

const PlayersTable: React.FC<{ result: BacktestEvaluateResponse }> = ({ result }) => {
  const anyWickets = result.players?.some((p) => typeof p.predicted?.wickets === 'number' || typeof p.actual?.wickets === 'number');
  const anyEconomy = result.players?.some((p) => typeof p.predicted?.economy === 'number' || typeof p.actual?.economy === 'number');
  const anyCatches = result.players?.some((p) => typeof p.predicted?.['catches'] === 'number' || typeof p.actual?.['catches'] === 'number');
  const anyRunOuts = result.players?.some((p) => typeof p.predicted?.['run_outs'] === 'number' || typeof p.actual?.['run_outs'] === 'number');

  return (
    <div style={{ maxHeight: 320, overflow: 'auto', border: '1px solid #eee', padding: 8 }}>
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
              <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{(p.predicted as any)?.runs?.toFixed?.(2) ?? (p.predicted as any)?.runs}</td>
              <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{(p.actual as any)?.runs?.toFixed?.(2) ?? (p.actual as any)?.runs}</td>
              <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{(p.errors as any)?.runs_mae?.toFixed?.(2) ?? (p.errors as any)?.runs_mae}</td>
              {anyWickets && (
                <>
                  <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{(p.predicted as any)?.wickets?.toFixed?.(2) ?? (p.predicted as any)?.wickets}</td>
                  <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{(p.actual as any)?.wickets?.toFixed?.(2) ?? (p.actual as any)?.wickets}</td>
                  <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{(p.errors as any)?.wickets_mae?.toFixed?.(2) ?? (p.errors as any)?.wickets_mae}</td>
                </>
              )}
              {anyEconomy && (
                <>
                  <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{(p.predicted as any)?.economy?.toFixed?.(2) ?? (p.predicted as any)?.economy}</td>
                  <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{(p.actual as any)?.economy?.toFixed?.(2) ?? (p.actual as any)?.economy}</td>
                  <td style={{ borderBottom: '1px solid #f0f0f0', padding: 6, textAlign: 'right' }}>{(p.errors as any)?.economy_mae?.toFixed?.(2) ?? (p.errors as any)?.economy_mae}</td>
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
    </div>
  );
};

const EvaluationResults: React.FC<EvaluationResultsProps> = ({ result }) => {
  return (
    <div>
      <div style={{ marginBottom: 8 }}>
        Match: <strong>{result.match.match_id}</strong> · Date: <strong>{new Date(result.match.date).toISOString().slice(0,10)}</strong>
      </div>
      <MetricsLine metrics={result.metrics} />
      <MatchAggregates aggregates={result.match_aggregates} />
      <PlayersTable result={result} />
    </div>
  );
};

export default EvaluationResults;
