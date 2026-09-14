import { useMemo, useState } from 'react';
import { api } from '../api';
import { useAsync } from './useAsync';
import type { EvaluationReport } from '../types';

/**
 * L4's evaluation report, and which format is being looked at.
 *
 * There is nothing to configure: the harness decides the folds and the locked window, and
 * writes one file. The tab picks a format to read and nothing else — a form
 * that let the browser choose a cutoff would be a choice made off the locked window, which
 * H-19 forbids.
 */
export function useEvaluationReport() {
  const report = useAsync(api.evaluationReport, {
    runOnMount: [],
    errorMessage: 'Could not load the evaluation report',
  });
  const [selectedFormat, setSelectedFormat] = useState('');

  const data = report.data as EvaluationReport | null;
  const formats = useMemo(() => Object.keys(data?.formats ?? {}), [data]);
  const format = selectedFormat && formats.includes(selectedFormat) ? selectedFormat : formats[0];

  return {
    report: data,
    loading: report.loading,
    error: report.error,
    reload: report.run,
    formats,
    format,
    setFormat: setSelectedFormat,
    formatReport: format ? (data?.formats?.[format] ?? null) : null,
  };
}
