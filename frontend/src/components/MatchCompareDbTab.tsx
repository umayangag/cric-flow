import React, { useMemo, useState } from "react";
import { api } from "../api";
import type { MatchSquadsResponse } from "../types";

const formats = ["", "TEST", "ODI", "T20I", "T20"] as const;

const MatchCompareDbTab: React.FC = () => {
  const today = useMemo(() => new Date().toISOString().slice(0, 10), []);
  const [matchId, setMatchId] = useState<string>("");
  const [asof, setAsof] = useState<string>(today);
  const [format, setFormat] = useState<string>("");
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<{
    teams: [string, string];
    probs: [number, number];
    actualWinnerIndex: 0 | 1 | null;
    predictedWinnerIndex: 0 | 1 | null;
    squads: MatchSquadsResponse["squads"];
  } | null>(null);

  const canCompare = Boolean(matchId && asof) && !loading;

  const onCompare = async () => {
    if (!matchId || !asof) return;
    try {
      setLoading(true);
      setError(null);
      setResult(null);

      const data = await api.getMatchSquads(matchId, asof, format || undefined);
      if (!data.squads || data.squads.length !== 2) {
        throw new Error("Unexpected squads response");
      }

      const probs: [number, number] = [0, 0];
      // Predict for each team
      for (let i = 0; i < 2; i++) {
        const resp = await api.predictWin(data.squads[i].players);
        probs[i] = resp.team_win_probability;
      }

      // Determine winners
      const predictedWinnerIndex =
        probs[0] === probs[1] ? null : probs[0] > probs[1] ? 0 : 1;
      const actualWinnerIndex =
        data.squads[0].actual_win === 1
          ? 0
          : data.squads[1].actual_win === 1
            ? 1
            : null;

      setResult({
        teams: data.teams,
        probs,
        actualWinnerIndex: actualWinnerIndex as 0 | 1 | null,
        predictedWinnerIndex: predictedWinnerIndex as 0 | 1 | null,
        squads: data.squads,
      });
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      // Friendly messages for common errors
      if (msg.includes("HTTP 404")) {
        setError("Match not found. Please verify the match ID.");
      } else if (msg.includes("HTTP 422")) {
        setError(
          "Incomplete squads for this match (one or both teams incomplete).",
        );
      } else {
        setError(msg);
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <p style={{ marginTop: 0 }}>
        Compare ML prediction vs actual result for a specific match from DB
        squads. Enter a match ID, select an as-of date and optional format, then
        click Compare.
      </p>

      <div
        style={{
          display: "flex",
          gap: 12,
          alignItems: "center",
          flexWrap: "wrap",
          marginBottom: 12,
        }}
      >
        <label>
          Match ID:{" "}
          <input
            type="number"
            value={matchId}
            onChange={(e) => setMatchId(e.target.value)}
            placeholder="e.g., 12345"
            aria-label="match-id-input"
            style={{ width: 120 }}
          />
        </label>
        <label>
          As of:{" "}
          <input
            type="date"
            value={asof}
            onChange={(e) => setAsof(e.target.value)}
            aria-label="asof-input"
          />
        </label>
        <label>
          Format:{" "}
          <select
            value={format}
            onChange={(e) => setFormat(e.target.value)}
            aria-label="format-select"
          >
            {formats.map((f) => (
              <option key={f} value={f}>
                {f === "" ? "All" : f}
              </option>
            ))}
          </select>
        </label>
        <button
          onClick={onCompare}
          disabled={!canCompare}
          aria-label="compare-button"
        >
          {loading ? "Comparing…" : "Compare"}
        </button>
      </div>

      {error && (
        <div style={{ color: "red", marginBottom: 12 }} role="alert">
          {error}
        </div>
      )}

      {!result ? (
        <div style={{ color: "#666" }}>No comparison yet.</div>
      ) : (
        <div>
          <h3 style={{ margin: "8px 0" }}>Comparison</h3>
          <div>
            <strong>{result.teams[0]}</strong>:{" "}
            {(result.probs[0] * 100).toFixed(2)}% &nbsp; | &nbsp;
            <strong>{result.teams[1]}</strong>:{" "}
            {(result.probs[1] * 100).toFixed(2)}%
          </div>
          <div style={{ marginTop: 8 }}>
            Predicted winner:{" "}
            {result.predictedWinnerIndex == null ? (
              <em>tie</em>
            ) : (
              <strong>{result.teams[result.predictedWinnerIndex]}</strong>
            )}
          </div>
          <div>
            Actual winner:{" "}
            {result.actualWinnerIndex == null ? (
              <em>unknown</em>
            ) : (
              <strong>{result.teams[result.actualWinnerIndex]}</strong>
            )}
          </div>
          {result.predictedWinnerIndex != null &&
            result.actualWinnerIndex != null && (
              <div style={{ marginTop: 8 }}>
                Outcome:{" "}
                {result.predictedWinnerIndex === result.actualWinnerIndex ? (
                  <span style={{ color: "#2e7d32" }}>Correct</span>
                ) : (
                  <span style={{ color: "#c62828" }}>Incorrect</span>
                )}
              </div>
            )}
        </div>
      )}
    </div>
  );
};

export default MatchCompareDbTab;
