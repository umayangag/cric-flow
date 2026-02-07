import React, { useCallback, useMemo, useState } from 'react';
import type { BacktestEvaluateResponse } from '../api/types';
import { fetchBacktestEvaluate } from '../api/client';

export type BacktestEvaluateProps = {
  baseUrl?: string;
  matchId: number;
  format: string;
  team1: string;
  team2: string;
  onResult?: (res: BacktestEvaluateResponse) => void;
};

function toUpperTrim(s: string): string {
  return (s || '').trim().toUpperCase();
}

function isRFC3339(s: string): boolean {
  const re = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;
  return re.test((s || '').trim());
}

export const BacktestEvaluate: React.FC<BacktestEvaluateProps> = ({
  baseUrl = '',
  matchId,
  format,
  team1,
  team2,
  onResult,
}) => {
  const [cutoff, setCutoff] = useState('');
  const [error, setError] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<BacktestEvaluateResponse | null>(null);

  const disabled = useMemo(() => loading, [loading]);

  const handleEvaluate = useCallback(async () => {
    setError('');
    setResult(null);
    const fmt = toUpperTrim(format);
    const t1 = toUpperTrim(team1);
    const t2 = toUpperTrim(team2);
    if (!fmt || !t1 || !t2 || !matchId || matchId <= 0) {
      setError('format, team1, team2, and a valid matchId are required');
      return;
    }
    if (!isRFC3339(cutoff)) {
      setError('cutoff must be RFC3339, e.g., 2024-10-30T14:00:00Z');
      return;
    }
    try {
      setLoading(true);
      const res = await fetchBacktestEvaluate(baseUrl, {
        format: fmt,
        team1: t1,
        team2: t2,
        matchId,
        cutoffUtcIso: cutoff,
        useML: true,
      });
      setResult(res);
      onResult && onResult(res);
    } catch (e: any) {
      setError(e?.message || 'Failed to evaluate');
    } finally {
      setLoading(false);
    }
  }, [baseUrl, cutoff, format, team1, team2, matchId, onResult]);

  const winnerAccuracy = (result?.metrics || {})['winner_accuracy'];
  const playerRunsMae = (result?.metrics || {})['player_runs_mae'];
  const predictedRuns = (result?.match_aggregates?.predicted || {})['runs'] as number | undefined;
  const actualRuns = (result?.match_aggregates?.actual || {})['runs'] as number | undefined;
  const modelVersion = (result?.filters || ({} as any))['model_version'] as string | undefined;

  return (
    <div>
      <h3>Evaluate Selected Match</h3>
      <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
        <label>
          Cutoff (UTC RFC3339)
          <input
            aria-label="cutoff"
            placeholder="2024-10-30T14:00:00Z"
            value={cutoff}
            onChange={(e) => setCutoff(e.target.value)}
          />
        </label>
        <button aria-label="evaluate" onClick={handleEvaluate} disabled={disabled}>
          {loading ? 'Evaluating...' : 'Evaluate'}
        </button>
      </div>
      {error && (
        <div role="alert" style={{ color: 'red', marginTop: 8 }}>
          {error}
        </div>
      )}
      {result && (
        <div style={{ marginTop: 16 }} aria-label="evaluation-summary">
          <div>Players: {Array.isArray(result.players) ? result.players.length : 0}</div>
          {typeof playerRunsMae === 'number' && <div>player_runs_mae: {playerRunsMae.toFixed(3)}</div>}
          {typeof winnerAccuracy === 'number' && (
            <div>winner_accuracy: {winnerAccuracy.toFixed(3)}</div>
          )}
          {typeof predictedRuns === 'number' && typeof actualRuns === 'number' && (
            <div>
              runs (pred vs actual): {predictedRuns} vs {actualRuns}
            </div>
          )}
          {modelVersion && <div>model_version: {modelVersion}</div>}
        </div>
      )}
    </div>
  );
};

export default BacktestEvaluate;
