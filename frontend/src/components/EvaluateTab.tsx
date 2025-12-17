import React, { useMemo, useState } from 'react';
import { api } from '../api';
import { buildSquads, computeMetrics, determineImmediateNextSeason, parseCsv } from '../utils/eval';

const EvaluateTab: React.FC = () => {
  const [csvText, setCsvText] = useState<string>('');
  const [cutoffDate, setCutoffDate] = useState<string>('2022-12-31');
  const [threshold, setThreshold] = useState<number>(0.5);
  const [status, setStatus] = useState<string>('');
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ReturnType<typeof computeMetrics> | null>(null);

  const onFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const text = await file.text();
    setCsvText(text);
  };

  const canRun = useMemo(() => Boolean(csvText && cutoffDate), [csvText, cutoffDate]);

  const runEval = async () => {
    try {
      setError(null);
      setResult(null);
      setStatus('Parsing CSV…');
      const rows = parseCsv(csvText);
      if (!rows.length) throw new Error('CSV has no rows');
      const squadsMap = buildSquads(rows);
      const squads = Array.from(squadsMap.values());
      const seasons = squads.map((s) => s.season);
      const nextSeason = determineImmediateNextSeason(cutoffDate, seasons);
      if (nextSeason == null) throw new Error('Could not determine immediate next season after cutoff.');
      const testSquads = squads.filter((s) => s.season === nextSeason);
      if (!testSquads.length) throw new Error(`No squads found in season ${nextSeason}.`);

      const preds: Array<{ prob: number; actual: number }> = [];
      let done = 0;
      for (const sq of testSquads) {
        setStatus(`Predicting ${done + 1}/${testSquads.length}…`);
        const resp = await api.predictWin(sq.players);
        preds.push({ prob: resp.team_win_probability, actual: sq.actual_win });
        done++;
      }
      const metrics = computeMetrics(preds, threshold);
      setResult(metrics);
      setStatus(`Done. Evaluated ${metrics.total} squads from season ${nextSeason}.`);
    } catch (e: any) {
      setError(e?.message || String(e));
      setStatus('');
    }
  };

  return (
    <div>
      <p style={{ marginTop: 0 }}>
        Upload a CSV with player-level rows and select a cutoff date X. The app evaluates the immediate next season.
      </p>
      <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
        <input type="file" accept=".csv,text/csv" onChange={onFileChange} />
        <label>
          Cutoff date (YYYY-MM-DD):{' '}
          <input value={cutoffDate} onChange={(e) => setCutoffDate(e.target.value)} placeholder="YYYY-MM-DD" />
        </label>
        <label>
          Threshold:{' '}
          <input
            type="number"
            step={0.01}
            min={0}
            max={1}
            value={threshold}
            onChange={(e) => setThreshold(Number(e.target.value))}
            style={{ width: 80 }}
          />
        </label>
        <button disabled={!canRun} onClick={runEval}>
          Run Evaluation
        </button>
      </div>

      {status && <div style={{ marginTop: 8 }}>{status}</div>}
      {error && <div style={{ marginTop: 8, color: 'red' }}>Error: {error}</div>}
      {result && (
        <div style={{ marginTop: 12 }}>
          <div>
            Total: <strong>{result.total}</strong>, Correct: <strong>{result.correct}</strong>, Accuracy:{' '}
            <strong>{(result.accuracy * 100).toFixed(2)}%</strong>
          </div>
          <div style={{ marginTop: 8 }}>
            Confusion Matrix (threshold {threshold}):
            <ul>
              <li>TP: {result.confusion.tp}</li>
              <li>TN: {result.confusion.tn}</li>
              <li>FP: {result.confusion.fp}</li>
              <li>FN: {result.confusion.fn}</li>
            </ul>
          </div>
        </div>
      )}

      <details style={{ marginTop: 12 }}>
        <summary>CSV Schema</summary>
        <pre style={{ background: '#f7f7f7', padding: 12, overflow: 'auto' }}>{`Required columns per row:
match_id,date,season(optional),team_name,actual_win,
player_name,runs_scored,balls_faced,fours_scored,sixes_scored,batting_position,strike_rate,
runs_conceded,deliveries,wickets_taken,econ
`}</pre>
      </details>
    </div>
  );
};

export default EvaluateTab;
