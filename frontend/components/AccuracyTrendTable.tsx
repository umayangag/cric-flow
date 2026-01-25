import React from "react";

export interface AccuracyTrendTableRow {
  match_id: number;
  date: string;
  format: string;
  team1: string;
  team2: string;
  metrics: Record<string, number>;
}

export interface AccuracyTrendTableProps {
  rows: AccuracyTrendTableRow[];
}

function fmt(v: number | undefined, digits = 2): string {
  if (v === undefined || v === null || Number.isNaN(v)) return "";
  return Number(v).toFixed(digits);
}

export const AccuracyTrendTable: React.FC<AccuracyTrendTableProps> = ({
  rows,
}) => {
  const hasPlayerMAE = rows.some(
    (r) => typeof r.metrics?.player_runs_mae === "number",
  );
  const hasTeamMAE = rows.some(
    (r) => typeof r.metrics?.team_runs_mae === "number",
  );
  const hasWinnerAcc = rows.some(
    (r) => typeof r.metrics?.team_winner_accuracy === "number",
  );

  if (!rows || rows.length === 0) {
    return <div style={{ padding: 8, color: "#666" }}>No results.</div>;
  }

  return (
    <div style={{ overflowX: "auto" }}>
      <table style={{ width: "100%", borderCollapse: "collapse" }}>
        <thead>
          <tr>
            <th style={th}>Date</th>
            <th style={th}>Format</th>
            <th style={th}>Team 1</th>
            <th style={th}>Team 2</th>
            {hasPlayerMAE && <th style={th}>player_runs_mae</th>}
            {hasTeamMAE && <th style={th}>team_runs_mae</th>}
            {hasWinnerAcc && <th style={th}>team_winner_accuracy</th>}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.match_id}>
              <td style={td}>{new Date(r.date).toLocaleString()}</td>
              <td style={td}>{r.format}</td>
              <td style={td}>{r.team1}</td>
              <td style={td}>{r.team2}</td>
              {hasPlayerMAE && (
                <td style={tdRight}>{fmt(r.metrics?.player_runs_mae)}</td>
              )}
              {hasTeamMAE && (
                <td style={tdRight}>{fmt(r.metrics?.team_runs_mae)}</td>
              )}
              {hasWinnerAcc && (
                <td style={tdRight}>
                  {fmt(r.metrics?.team_winner_accuracy, 0)}
                </td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

const th: React.CSSProperties = {
  textAlign: "left",
  borderBottom: "1px solid #ddd",
  padding: "8px 6px",
  whiteSpace: "nowrap",
};
const td: React.CSSProperties = {
  borderBottom: "1px solid #f0f0f0",
  padding: "6px",
  whiteSpace: "nowrap",
};
const tdRight: React.CSSProperties = { ...td, textAlign: "right" };

export default AccuracyTrendTable;
