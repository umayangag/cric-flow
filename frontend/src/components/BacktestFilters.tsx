import React, { useCallback, useEffect, useMemo, useState } from 'react';
import type { BacktestCandidate, BacktestSelectResponse } from '../api/types';
import {
  fetchBacktestSelect,
  fetchFormats,
  fetchOpponents,
  fetchTeamsByFormat,
  toUpperTrim,
} from '../api/client';

export type BacktestFiltersProps = {
  baseUrl?: string;
  onSelect: (matchId: number, candidate: BacktestCandidate) => void;
};

export const BacktestFilters: React.FC<BacktestFiltersProps> = ({ baseUrl = '', onSelect }) => {
  const [format, setFormat] = useState('');
  const [team1, setTeam1] = useState('');
  const [team2, setTeam2] = useState('');
  const [error, setError] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [candidates, setCandidates] = useState<BacktestCandidate[]>([]);

  const [availableFormats, setAvailableFormats] = useState<string[]>([]);
  const [availableTeam1s, setAvailableTeam1s] = useState<string[]>([]);
  const [availableTeam2s, setAvailableTeam2s] = useState<string[]>([]);

  // Fetch formats on mount
  useEffect(() => {
    fetchFormats(baseUrl)
      .then((fmts) => {
        setAvailableFormats(fmts);
        if (fmts.length > 0 && !format) {
          setFormat(fmts[0]);
        }
      })
      .catch((err) => setError(err.message || 'Failed to fetch formats'));
    // format excluded: only used to avoid overwriting user selection on initial load
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [baseUrl]);

  // Fetch teams when format changes
  useEffect(() => {
    if (!format) {
      setAvailableTeam1s([]);
      return;
    }
    fetchTeamsByFormat(baseUrl, format)
      .then((teams) => {
        setAvailableTeam1s(teams);
        if (teams.length > 0) {
          setTeam1(teams[0]);
        } else {
          setTeam1('');
        }
      })
      .catch((err) => setError(err.message || 'Failed to fetch teams'));
  }, [baseUrl, format]);

  // Fetch opponents when team1 or format changes
  useEffect(() => {
    if (!format || !team1) {
      setAvailableTeam2s([]);
      return;
    }
    fetchOpponents(baseUrl, format, team1)
      .then((opps) => {
        setAvailableTeam2s(opps);
        if (opps.length > 0) {
          setTeam2(opps[0]);
        } else {
          setTeam2('');
        }
      })
      .catch((err) => setError(err.message || 'Failed to fetch opponents'));
  }, [baseUrl, format, team1]);

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
          <select
            aria-label="format"
            value={format}
            onChange={(e) => setFormat(e.target.value)}
            disabled={disabled}
            style={{ padding: '4px 8px' }}
          >
            <option value="">Select Format</option>
            {availableFormats.map((f) => (
              <option key={f} value={f}>
                {f}
              </option>
            ))}
          </select>
        </label>
        <label>
          Team 1
          <select
            aria-label="team1"
            value={team1}
            onChange={(e) => setTeam1(e.target.value)}
            disabled={disabled || !format}
            style={{ padding: '4px 8px' }}
          >
            <option value="">Select Team 1</option>
            {availableTeam1s.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </label>
        <label>
          Team 2
          <select
            aria-label="team2"
            value={team2}
            onChange={(e) => setTeam2(e.target.value)}
            disabled={disabled || !team1}
            style={{ padding: '4px 8px' }}
          >
            <option value="">Select Team 2</option>
            {availableTeam2s.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </label>
        <button
          onClick={handleSearch}
          disabled={disabled || !format || !team1 || !team2}
          aria-label="search"
          style={{ padding: '4px 12px' }}
        >
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
