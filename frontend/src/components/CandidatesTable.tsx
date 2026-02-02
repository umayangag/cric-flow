import React from "react";
import type { BacktestCandidate } from "../types";

type CandidatesTableProps = {
  candidates: BacktestCandidate[];
  selectedMatchId: number | null;
  onSelectMatch: (matchId: number) => void;
};

const styles: { [key: string]: React.CSSProperties } = {
  emptyState: { color: "#666" },
  container: {
    maxHeight: 240,
    overflow: "auto",
    border: "1px solid #eee",
    padding: 8,
  },
  table: { width: "100%", borderCollapse: "collapse" },
  th: { textAlign: "left", borderBottom: "1px solid #ddd", padding: 6 },
  td: { borderBottom: "1px solid #f0f0f0", padding: 6 },
};

const CandidatesTable: React.FC<CandidatesTableProps> = ({
  candidates,
  selectedMatchId,
  onSelectMatch,
}) => {
  if (!candidates.length) {
    return <div style={styles.emptyState}>No candidates loaded yet.</div>;
  }
  return (
    <div style={styles.container}>
      <table style={styles.table}>
        <thead>
          <tr>
            <th style={styles.th}>Date</th>
            <th style={styles.th}>Match</th>
            <th style={styles.th}>Venue</th>
            <th style={styles.th}>Winner</th>
            <th style={styles.th}>Action</th>
          </tr>
        </thead>
        <tbody>
          {candidates.map((c) => (
            <tr key={c.match_id}>
              <td style={styles.td}>
                {new Date(c.date).toISOString().slice(0, 10)}
              </td>
              <td style={styles.td}>
                {c.team1} vs {c.team2}
              </td>
              <td style={styles.td}>{c.venue || "-"}</td>
              <td style={styles.td}>{c.winner_team_code || "-"}</td>
              <td style={styles.td}>
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
