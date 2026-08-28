import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import EvaluationProvenance from './EvaluationProvenance';
import type { ModelStatsResponse } from '../types';

const stats: ModelStatsResponse = {
  models_dir: '/models',
  models: [
    {
      model_name: 'batting_model_T20I',
      match_format: 'T20I',
      trained_at: '2026-08-01T00:00:00Z',
      provenance: { dataset_sha256: 'a'.repeat(64), training_cutoff: '2026-07-01T00:00:00Z' },
      dataset_is_live: false,
    },
    {
      model_name: 'bowling_model_ODI',
      match_format: 'ODI',
      dataset_is_live: true,
    },
    {
      // Trained before provenance existed: no dataset, no verdict.
      model_name: 'win_model_T20I',
      match_format: 'T20I',
    },
  ],
};

describe('EvaluationProvenance', () => {
  it('shows only the models for the evaluated format', () => {
    render(<EvaluationProvenance format="T20I" stats={stats} loading={false} />);
    expect(screen.getByText('batting_model_T20I')).toBeInTheDocument();
    expect(screen.getByText('win_model_T20I')).toBeInTheDocument();
    expect(screen.queryByText('bowling_model_ODI')).not.toBeInTheDocument();
  });

  /**
   * The whole reason this panel exists: a number scored by a model trained on data
   * that is no longer here may have been scored against its own training set.
   */
  it('flags a model trained on a dataset that is no longer live', () => {
    render(<EvaluationProvenance format="T20I" stats={stats} loading={false} />);
    expect(screen.getByText(/trained on other data/)).toBeInTheDocument();
  });

  it('says unknown rather than no when there is nothing to compare', () => {
    render(<EvaluationProvenance format="T20I" stats={stats} loading={false} />);
    expect(screen.getByText('unknown')).toBeInTheDocument();
  });

  it('does not present a missing record as a clean one', () => {
    render(<EvaluationProvenance format="TEST" stats={stats} loading={false} />);
    expect(screen.getByText(/unknown, not clean/)).toBeInTheDocument();
  });
});
