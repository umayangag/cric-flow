import React, { useMemo, useState } from "react";
import { api } from "../api";
import {
  buildSquads,
  computeMetrics,
  determineImmediateNextSeason,
  parseCsv,
} from "../utils/eval";
import EvaluationMetricsSummary from "./EvaluationMetricsSummary";

const EvaluateTab: React.FC = () => {
  const [csvText, setCsvText] = useState<string>("");
  const [cutoffDate, setCutoffDate] = useState<string>("2022-12-31");
  const [threshold, setThreshold] = useState<number>(0.5);
  const [statusMessage, setStatusMessage] = useState<string>("");
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [evaluationMetrics, setEvaluationMetrics] = useState<ReturnType<
    typeof computeMetrics
  > | null>(null);

  const handleCsvFileChange = async (
    e: React.ChangeEvent<HTMLInputElement>,
  ) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const text = await file.text();
    setCsvText(text);
  };

  const canRunEvaluation = useMemo(
    () => Boolean(csvText && cutoffDate),
    [csvText, cutoffDate],
  );

  const runEvaluation = async () => {
    try {
      setErrorMessage(null);
      setEvaluationMetrics(null);
      setStatusMessage("Parsing CSV…");
      const rows = parseCsv(csvText);
      if (!rows.length) throw new Error("CSV has no rows");
      const squadsMap = buildSquads(rows);
      const squads = Array.from(squadsMap.values());
      const seasons = squads.map((s) => s.season);
      const nextSeason = determineImmediateNextSeason(cutoffDate, seasons);
      if (nextSeason == null)
        throw new Error(
          "Could not determine immediate next season after cutoff.",
        );
      const testSquads = squads.filter((s) => s.season === nextSeason);
      if (!testSquads.length)
        throw new Error(`No squads found in season ${nextSeason}.`);

      setStatusMessage(`Predicting for ${testSquads.length} squads...`);
      const responses = await Promise.all(
        testSquads.map((sq) => api.predictWin(sq.players)),
      );
      const preds: Array<{ prob: number; actual: number }> = responses.map(
        (resp, i) => ({
          prob: resp.team_win_probability,
          actual: testSquads[i].actual_win,
        }),
      );
      const metrics = computeMetrics(preds, threshold);
      setEvaluationMetrics(metrics);
      setStatusMessage(
        `Done. Evaluated ${metrics.total} squads from season ${nextSeason}.`,
      );
    } catch (e: unknown) {
      setErrorMessage(e instanceof Error ? e.message : String(e));
      setStatusMessage("");
    }
  };

  return (
    <div>
      <p style={{ marginTop: 0 }}>
        Upload a CSV with player-level rows and select a cutoff date X. The app
        evaluates the immediate next season.
      </p>
      <div
        style={{
          display: "flex",
          gap: 12,
          alignItems: "center",
          flexWrap: "wrap",
        }}
      >
        <input
          type="file"
          accept=".csv,text/csv"
          onChange={handleCsvFileChange}
        />
        <label>
          Cutoff date (YYYY-MM-DD):{" "}
          <input
            value={cutoffDate}
            onChange={(e) => setCutoffDate(e.target.value)}
            placeholder="YYYY-MM-DD"
          />
        </label>
        <label>
          Threshold:{" "}
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
        <button disabled={!canRunEvaluation} onClick={runEvaluation}>
          Run Evaluation
        </button>
      </div>

      {statusMessage && <div style={{ marginTop: 8 }}>{statusMessage}</div>}
      {errorMessage && (
        <div style={{ marginTop: 8, color: "red" }}>Error: {errorMessage}</div>
      )}
      {evaluationMetrics && (
        <div style={{ marginTop: 12 }}>
          <EvaluationMetricsSummary
            metrics={evaluationMetrics}
            threshold={threshold}
          />
        </div>
      )}

      <details style={{ marginTop: 12 }}>
        <summary>CSV Schema</summary>
        <pre
          style={{ background: "#f7f7f7", padding: 12, overflow: "auto" }}
        >{`Required columns per row:
match_id,date,season(optional),team_name,actual_win,
player_name,runs_scored,balls_faced,fours_scored,sixes_scored,batting_position,strike_rate,
runs_conceded,deliveries,wickets_taken,econ
`}</pre>
      </details>
    </div>
  );
};

export default EvaluateTab;
