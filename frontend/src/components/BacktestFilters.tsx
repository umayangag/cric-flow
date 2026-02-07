import React, { useCallback, useMemo, useState } from 'react';
import type { BacktestCandidate, BacktestSelectResponse } from '../api/types';
import { fetchBacktestSelect } from '../api/client';

export type BacktestFiltersProps = {
  baseUrl?: string;
  onSelect: (matchId: number, candidate: BacktestCandidate) => void;
};

function toUpperTrim(s: string): string {
  return (s || '').trim().toUpperCase();
}

export const BacktestFilters: React.FC<BacktestFiltersProps> = ({ baseUrl = '', onSelect }) => {
  const [format, setFormat] = useState('T20');
  const [team1, setTeam1] = useState('IND');
  const [team2, setTeam2] = useState('AUS');
  const [error, setError] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [candidates, setCandidates] = useState<BacktestCandidate[]>([]);

  const disabled = useMemo(() => loading, [loading]);

  const handleSearch = useCallback(async () => {
    setError('');
    setCandidates([]);
    const f = toUpperTrim(format);
    const t1 = toUpperTrim(team1);
    const t2 = toUpperTrim(team2);
    if (!f || !t1 || !t2) {
      setError('format, team1, and team2 are required');
      return;
    }
    try {
      setLoading(true);
      const res: BacktestSelectResponse = await fetchBacktestSelect(baseUrl, {
        format: f,
        team1: t1,
        team2: t2,
      });
      setCandidates(res.candidates || []);
    } catch (e: unknown) {
      const err = e as { message?: string } | undefined;
      setError(err?.message || 'Failed to fetch candidates');
    } finally {
      setLoading(false);
    }
  }, [baseUrl, format, team1, team2]);

  return (
    <div>
      <h2>Historical Backtest - Filters</h2>
      <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
        <label>
          Format
          <input
            aria-label="format"
            value={format}
            onChange={(e) => setFormat(e.target.value)}
            placeholder="T20"
          />
        </label>
        <label>
          Team 1
          <input
            aria-label="team1"
            value={team1}
            onChange={(e) => setTeam1(e.target.value)}
            placeholder="IND"
          />
        </label>
        <label>
          Team 2
          <input
            aria-label="team2"
            value={team2}
            onChange={(e) => setTeam2(e.target.value)}
            placeholder="AUS"
          />
        </label>
        <button onClick={handleSearch} disabled={disabled} aria-label="search">
          {loading ? 'Searching...' : 'Search'}
        </button>
      </div>
      {error && (
        <div role="alert" style={{ color: 'red', marginTop: 8 }}>
          {error}
        </div>
      )}
      <div style={{ marginTop: 16 }}>
        {candidates.length > 0 ? (
          <table
            aria-label="candidates-table"
            style={{ borderCollapse: 'collapse', width: '100%' }}
          >
            <thead>
              <tr>
                <th style={{ textAlign: 'left' }}>Match ID</th>
                <th style={{ textAlign: 'left' }}>Date</th>
                <th style={{ textAlign: 'left' }}>Venue</th>
                <th style={{ textAlign: 'left' }}>Winner</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {candidates.map((c) => (
                <tr key={c.match_id}>
                  <td>{c.match_id}</td>
                  <td>{c.date}</td>
                  <td>{c.venue}</td>
                  <td>{c.winner_team_code}</td>
                  <td>
                    <button
                      aria-label={`select-${c.match_id}`}
                      onClick={() => onSelect(c.match_id, c)}
                    >
                      Select
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          !loading && <div>No candidates yet. Enter filters and Search.</div>
        )}
      </div>
    </div>
  );
};

export default BacktestFilters;
