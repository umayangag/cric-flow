import React from 'react';
import type { BacktestCandidate } from '../types';

type CandidatesTableProps = {
  candidates: BacktestCandidate[];
  selectedMatchId: number | null;
  onSelectMatch: (matchId: number) => void;
};

const CandidatesTable: React.FC<CandidatesTableProps> = ({ candidates, selectedMatchId, onSelectMatch }) => {
  if (!candidates.length) {
    return <div style={{ color: '#666' }}>No candidates loaded yet.</div>;
  }
  return (
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
                    onChange={() => onSelectMatch(c.match_id)}
                  />
                  &nbsp;Select
                </label>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

export default CandidatesTable;
