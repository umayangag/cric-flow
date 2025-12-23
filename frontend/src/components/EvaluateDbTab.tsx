import React, { useMemo, useState } from 'react';
import { api } from '../api';
import type { MatchListItem, MatchSquadsResponse } from '../types';
import { computeMetrics } from '../utils/eval';

type MatchUI = MatchListItem & { ui_status: 'Pending' | 'Loaded' | 'Evaluated' | 'Error' };

const formats = ['', 'TEST', 'ODI', 'T20I', 'T20'] as const;

const EvaluateDbTab: React.FC = () => {
  const today = useMemo(() => new Date().toISOString().slice(0, 10), []);
  const [cutoff, setCutoff] = useState<string>(today);
  const [format, setFormat] = useState<string>('');
  const [season, setSeason] = useState<number | null>(null);
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [matches, setMatches] = useState<MatchUI[]>([]);
  const [status, setStatus] = useState<string>('');
  const [evaluating, setEvaluating] = useState<boolean>(false);
  const [progress, setProgress] = useState<{ done: number; total: number }>({ done: 0, total: 0 });
  const [metrics, setMetrics] = useState<ReturnType<typeof computeMetrics> | null>(null);

  const canLoad = useMemo(() => Boolean(cutoff) && !loading, [cutoff, loading]);
  const canEvaluate = useMemo(() => matches.length > 0, [matches.length]);

  // Reset hygiene: when cutoff or format changes, clear loaded state so user must reload
  const onCutoffChange = (v: string) => {
    setCutoff(v);
    setMatches([]);
    setMetrics(null);
    setSeason(null);
    setStatus('');
    setError(null);
  };
  const onFormatChange = (v: string) => {
    setFormat(v);
    setMatches([]);
    setMetrics(null);
    setSeason(null);
    setStatus('');
    setError(null);
  };

  const onLoadMatches = async () => {
    try {
      setLoading(true);
      setError(null);
      setStatus('Determining next season…');
      setMatches([]);
      setSeason(null);
      const next = await api.seasonsNext(cutoff, format || undefined);
      if (next.next_season == null) {
        setStatus('No next season after cutoff.');
        return;
      }
      setSeason(next.next_season);
      setStatus(`Loading matches for season ${next.next_season}…`);
      const list = await api.listMatches(next.next_season, cutoff, format || undefined);
      const ui: MatchUI[] = list.map((m) => ({ ...m, ui_status: 'Pending' }));
      setMatches(ui);
      setStatus(`Loaded ${ui.length} matches for season ${next.next_season}.`);
    } catch (e: any) {
      setError(e?.message || String(e));
      setStatus('');
    } finally {
      setLoading(false);
    }
  };

  const onEvaluate = async () => {
    if (!matches.length) return;
    setEvaluating(true);
    setError(null);
    setStatus('Evaluating matches…');
    setProgress({ done: 0, total: matches.length });
    setMetrics(null);

    // Make a shallow copy to mutate UI status safely
    setMatches((prev) => prev.map((m) => ({ ...m, ui_status: 'Loaded' })));

    const preds: Array<{ prob: number; actual: number }> = [];

    const worker = async (m: MatchUI) => {
      try {
        const resp: MatchSquadsResponse = await api.getMatchSquads(m.match_id, cutoff, format || undefined);
        if (!resp.squads || resp.squads.length !== 2) {
          throw new Error('unexpected squads response');
        }
        // Two teams; run predictions per team and push team-level preds
        for (const squad of resp.squads) {
          const win = await api.predictWin(squad.players);
          preds.push({ prob: win.team_win_probability, actual: Number(squad.actual_win) });
        }
        // mark evaluated
        setMatches((prev) => prev.map((x) => (x.match_id === m.match_id ? { ...x, ui_status: 'Evaluated' } : x)));
      } catch (e: any) {
        setMatches((prev) => prev.map((x) => (x.match_id === m.match_id ? { ...x, ui_status: 'Error' } : x)));
      } finally {
        setProgress((p) => ({ ...p, done: p.done + 1 }));
      }
    };

    const limit = 5;
    for (let start = 0; start < matches.length; start += limit) {
      const slice = matches.slice(start, Math.min(start + limit, matches.length));
      await Promise.all(slice.map((m) => worker(m)));
    }

    const mtr = computeMetrics(preds, 0.5);
    setMetrics(mtr);
    setStatus('Evaluation complete.');
    setEvaluating(false);
  };

  return (
    <div>
      <p style={{ marginTop: 0 }}>
        Evaluate model accuracy using DB-backed matches and squads. Select a cutoff date and optional format, then load
        matches for the immediate next season after the cutoff.
      </p>

      <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap', marginBottom: 12 }}>
        <label>
          Cutoff date:{' '}
          <input type="date" value={cutoff} onChange={(e) => onCutoffChange(e.target.value)} />
        </label>
        <label>
          Format:{' '}
          <select value={format} onChange={(e) => onFormatChange(e.target.value)}>
            {formats.map((f) => (
              <option key={f} value={f}>
                {f === '' ? 'All' : f}
              </option>
            ))}
          </select>
        </label>
        <button onClick={onLoadMatches} disabled={!canLoad}>
          Load Matches
        </button>
        <button onClick={onEvaluate} disabled={!canEvaluate || evaluating || loading}>
          Evaluate
        </button>
        {season != null && (
          <span style={{ marginLeft: 12, color: '#555' }} aria-label="selection-context">
            Cutoff: {cutoff} · Format: {format || 'All'} · Season: {season}
          </span>
        )}
      </div>

      {status && (
        <div style={{ marginBottom: 12 }} aria-live="polite" aria-atomic="true">
          {status}
        </div>
      )}
      {error && (
        <div style={{ marginBottom: 12, color: 'red' }}>Error: {error}</div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        <section aria-label="matches-section">
          <h3 style={{ margin: '8px 0' }}>Matches</h3>
          {matches.length === 0 ? (
            <div style={{ color: '#666' }}>No matches loaded yet.</div>
          ) : (
            <div style={{ maxHeight: 280, overflow: 'auto', border: '1px solid #eee', padding: 8 }}>
              <ul style={{ margin: 0, paddingLeft: 16 }}>
                {matches.map((m) => {
                  const label = `${m.teams[0]} vs ${m.teams[1]} — ${m.date}`;
                  const badgeColor =
                    m.ui_status === 'Evaluated'
                      ? '#2e7d32'
                      : m.ui_status === 'Loaded'
                      ? '#1565c0'
                      : m.ui_status === 'Error'
                      ? '#c62828'
                      : '#757575';
                  return (
                    <li key={String(m.match_id)}>
                      <span
                        aria-label={`status-${m.ui_status.toLowerCase()}`}
                        style={{
                          display: 'inline-block',
                          minWidth: 8,
                          minHeight: 8,
                          borderRadius: 8,
                          background: badgeColor,
                          marginRight: 8,
                        }}
                      />
                      {label} — <em>{m.ui_status}</em>
                    </li>
                  );
                })}
              </ul>
            </div>
          )}
          {evaluating && (
            <div style={{ marginTop: 8 }}>
              Progress: {progress.done}/{progress.total}
              <div style={{ height: 6, background: '#eee', marginTop: 4 }}>
                <div
                  style={{
                    height: '100%',
                    width: `${progress.total ? Math.round((progress.done / progress.total) * 100) : 0}%`,
                    background: '#55f',
                  }}
                />
              </div>
            </div>
          )}
        </section>

        <section aria-label="results-section">
          <h3 style={{ margin: '8px 0' }}>Results</h3>
          {!metrics ? (
            <div style={{ color: '#666' }}>
              Accuracy and confusion matrix will appear here after evaluation.
            </div>
          ) : (
            <div>
              <div>
                Teams evaluated: <strong>{metrics.total}</strong> &nbsp; | Correct:{' '}
                <strong>{metrics.correct}</strong> &nbsp; | Accuracy:{' '}
                <strong>{(metrics.accuracy * 100).toFixed(2)}%</strong>
              </div>
              <div style={{ marginTop: 8 }}>
                <table style={{ borderCollapse: 'collapse' }} aria-label="confusion-matrix">
                  <thead>
                    <tr>
                      <th style={{ padding: 6 }}></th>
                      <th style={{ border: '1px solid #ddd', padding: 6 }}>Pred=1</th>
                      <th style={{ border: '1px solid #ddd', padding: 6 }}>Pred=0</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <td style={{ border: '1px solid #ddd', padding: 6 }}>Actual=1</td>
                      <td style={{ border: '1px solid #ddd', padding: 6 }}>TP: {metrics.confusion.tp}</td>
                      <td style={{ border: '1px solid #ddd', padding: 6 }}>FN: {metrics.confusion.fn}</td>
                    </tr>
                    <tr>
                      <td style={{ border: '1px solid #ddd', padding: 6 }}>Actual=0</td>
                      <td style={{ border: '1px solid #ddd', padding: 6 }}>FP: {metrics.confusion.fp}</td>
                      <td style={{ border: '1px solid #ddd', padding: 6 }}>TN: {metrics.confusion.tn}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </section>
      </div>
    </div>
  );
};

export default EvaluateDbTab;
