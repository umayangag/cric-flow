import React, { useCallback, useState } from 'react';
import type { BacktestCandidate, BacktestEvaluateResponse } from '../api/types';
import { BacktestFilters } from '../components/BacktestFilters';
import { BacktestEvaluate } from '../components/BacktestEvaluate';
import { BacktestResults } from '../components/BacktestResults';

export const BacktestPage: React.FC<{ baseUrl?: string }> = ({ baseUrl = '' }) => {
  const [selected, setSelected] = useState<BacktestCandidate | null>(null);
  const [lastResult, setLastResult] = useState<BacktestEvaluateResponse | null>(null);

  const handleSelect = useCallback((matchId: number, cand: BacktestCandidate) => {
    setSelected(cand);
    setLastResult(null);
  }, []);

  const handleResult = useCallback((res: BacktestEvaluateResponse) => {
    setLastResult(res);
  }, []);

  return (
    <div style={{ padding: 16 }}>
      <BacktestFilters baseUrl={baseUrl} onSelect={handleSelect} />

      {selected && (
        <div style={{ marginTop: 24 }}>
          <h3>Selected match: {selected.match_id}</h3>
          <div style={{ marginBottom: 8 }}>
            {selected.format} — {selected.team1} vs {selected.team2} — {selected.date}
          </div>
          <BacktestEvaluate
            baseUrl={baseUrl}
            matchId={selected.match_id}
            format={selected.format}
            team1={selected.team1}
            team2={selected.team2}
            onResult={handleResult}
          />
        </div>
      )}

      {lastResult && (
        <div style={{ marginTop: 24 }}>
          <BacktestResults result={lastResult} />
        </div>
      )}
    </div>
  );
};

export default BacktestPage;
