import React from 'react';
import type { EvalMetrics } from '../utils/eval';

type Props = {
  metrics: EvalMetrics;
  threshold: number;
};

const EvaluationMetricsSummary: React.FC<Props> = ({ metrics, threshold }) => {
  return (
    <div>
      <div>
        Total: <strong>{metrics.total}</strong>, Correct: <strong>{metrics.correct}</strong>,
        Accuracy: <strong>{(metrics.accuracy * 100).toFixed(2)}%</strong>
      </div>
      <div style={{ marginTop: 8 }}>
        Confusion Matrix (threshold {threshold}):
        <ul>
          <li>TP: {metrics.confusion.tp}</li>
          <li>TN: {metrics.confusion.tn}</li>
          <li>FP: {metrics.confusion.fp}</li>
          <li>FN: {metrics.confusion.fn}</li>
        </ul>
      </div>
    </div>
  );
};

export default EvaluationMetricsSummary;
