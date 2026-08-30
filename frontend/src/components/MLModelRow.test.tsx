import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import React from 'react';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import MLModelRow from './MLModelRow';
import type { MLModelStat } from '../types';

function model(overrides: Partial<MLModelStat>): MLModelStat {
  return {
    model_name: 'Batting',
    match_format: 'T20I',
    ...overrides,
  };
}

function renderRow(stat: MLModelStat) {
  return render(
    <Table>
      <TableBody>
        <MLModelRow model={stat} />
      </TableBody>
    </Table>,
  );
}

describe('MLModelRow score source', () => {
  afterEach(cleanup);

  // A holdout MAE and a tuned CV MAE share one column but are not comparable, so the
  // score is only legible next to how it was measured.
  it('labels a single-train score as a holdout', () => {
    renderRow(model({ accuracy_display: 'MAE=6.50', score_source: 'holdout' }));

    expect(screen.getByText('MAE=6.50')).toBeDefined();
    expect(screen.getByText('holdout')).toBeDefined();
  });

  it('labels an auto-tuned score as cross-validated', () => {
    renderRow(model({ accuracy_display: 'MAE=5.10', score_source: 'tuning_cv', tuned: true }));

    expect(screen.getByText('CV')).toBeDefined();
  });

  it('shows no label when the model carries no score source', () => {
    renderRow(model({ accuracy_display: 'MAE=6.50' }));

    expect(screen.queryByText('holdout')).toBeNull();
    expect(screen.queryByText('CV')).toBeNull();
  });

  // A chip beside an em dash would claim a measurement that was never taken.
  it('shows no label when there is no score to label', () => {
    renderRow(model({ score_source: 'holdout' }));

    expect(screen.queryByText('holdout')).toBeNull();
  });
});

describe('MLModelRow details toggle', () => {
  afterEach(cleanup);

  it('expands to reveal the details panel when the model has details', () => {
    renderRow(model({ metrics: { mae: 6.5 }, accuracy_display: 'MAE=6.50' }));

    fireEvent.click(screen.getByLabelText('show details'));

    expect(screen.getByText('Metrics')).toBeDefined();
    expect(screen.getByText('mae=6.5')).toBeDefined();
    expect(screen.getByLabelText('hide details')).toBeDefined();
  });

  it('collapses again on a second click', () => {
    renderRow(model({ metrics: { mae: 6.5 } }));

    fireEvent.click(screen.getByLabelText('show details'));
    fireEvent.click(screen.getByLabelText('hide details'));

    expect(screen.getByLabelText('show details')).toBeDefined();
  });

  // Nothing to expand into, so the control is not offered at all.
  it('offers no toggle for a model with no tuned params, metrics or audit', () => {
    renderRow(model({ accuracy_display: 'MAE=6.50' }));

    expect(screen.queryByLabelText('show details')).toBeNull();
  });
});
