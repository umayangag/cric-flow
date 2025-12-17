import React, { useState } from 'react';
import { api } from '../api';
import type { PlayerPrediction, TeamWinResponse } from '../types';

const defaultPlayers: PlayerPrediction[] = [
  {
    player_name: 'Player A',
    runs_scored: 30,
    balls_faced: 25,
    fours_scored: 3,
    sixes_scored: 1,
    batting_position: 3,
    strike_rate: 120,
    runs_conceded: 0,
    deliveries: 0,
    wickets_taken: 0,
    econ: 0,
  },
  {
    player_name: 'Player B',
    runs_scored: 0,
    balls_faced: 0,
    fours_scored: 0,
    sixes_scored: 0,
    batting_position: 7,
    strike_rate: 0,
    runs_conceded: 24,
    deliveries: 24,
    wickets_taken: 2,
    econ: 6,
  },
];

const PredictTab: React.FC = () => {
  const [input, setInput] = useState(JSON.stringify(defaultPlayers, null, 2));
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<TeamWinResponse | null>(null);

  const onSubmit = async () => {
    try {
      setLoading(true);
      setError(null);
      setResult(null);
      const players = JSON.parse(input) as PlayerPrediction[];
      const resp = await api.predictWin(players);
      setResult(resp);
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <p style={{ marginTop: 0 }}>Paste or edit a players array and run prediction.</p>
      <textarea
        value={input}
        onChange={(e) => setInput(e.target.value)}
        rows={14}
        style={{ width: '100%', fontFamily: 'monospace', fontSize: 12 }}
      />
      <div style={{ marginTop: 8 }}>
        <button onClick={onSubmit} disabled={loading}>
          {loading ? 'Predicting…' : 'Predict'}
        </button>
      </div>
      {error && <div style={{ color: 'red', marginTop: 8 }}>Error: {error}</div>}
      {result && (
        <div style={{ marginTop: 12 }}>
          <div>
            Team win probability: <strong>{result.team_win_probability.toFixed(3)}</strong>
          </div>
          <div style={{ marginTop: 8 }}>
            <details>
              <summary>Players (returned)</summary>
              <pre style={{ background: '#f7f7f7', padding: 12, overflow: 'auto' }}>
{JSON.stringify(result.players, null, 2)}
              </pre>
            </details>
          </div>
        </div>
      )}
    </div>
  );
};

export default PredictTab;
