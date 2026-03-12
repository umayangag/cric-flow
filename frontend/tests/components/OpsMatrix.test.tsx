import { render, screen } from '@testing-library/react';
import React from 'react';
import OpsMatrix from '../../src/components/OpsMatrix';

const CANONICAL_FORMATS = ['TEST', 'ODI', 'T20I', 'T20'];

describe('OpsMatrix', () => {
  it('renders precompute statuses per format', () => {
    const data = {
      formats: {
        TEST: { status: 'ok' },
        ODI: { status: 'stale' },
        T20I: { status: 'missing' },
        T20: { status: 'ok' },
      },
    };
    render(<OpsMatrix type="precompute" title="Precompute" data={data} formats={CANONICAL_FORMATS} />);
    expect(screen.getByTestId('precompute-TEST')).toBeInTheDocument();
    expect(screen.getByTestId('precompute-ODI')).toBeInTheDocument();
    expect(screen.getByTestId('precompute-T20I')).toBeInTheDocument();
    expect(screen.getByTestId('precompute-T20')).toBeInTheDocument();
    // Text content should contain the status keywords
    expect(screen.getByTestId('precompute-ODI').textContent).toMatch(/stale/i);
    expect(screen.getByTestId('precompute-T20I').textContent).toMatch(/missing/i);
  });

  it('renders exports presence per format', () => {
    const data = {
      formats: {
        TEST: { files: [{ name: 'batting_on.csv', exists: true }] },
        ODI: { files: [] },
        T20I: { files: [{ name: 'x.csv', exists: false }] },
        T20: { files: [{ name: 'y.csv', exists: true }] },
      },
    };
    render(<OpsMatrix type="exports" title="Exports" data={data} formats={CANONICAL_FORMATS} />);
    expect(screen.getByTestId('exports-TEST').textContent).toMatch(/present/i);
    expect(screen.getByTestId('exports-ODI').textContent).toMatch(/missing/i);
  });

  it('renders artifacts exists/loaded per format', () => {
    const data = {
      formats: {
        ODI: {
          batting: { exists: true, loaded: true },
          bowling: { exists: false },
        },
        T20: {
          batting: { exists: true },
          bowling: { exists: true },
        },
      },
    };
    render(<OpsMatrix type="artifacts" title="Artifacts" data={data} formats={CANONICAL_FORMATS} />);
    expect(screen.getByTestId('artifacts-ODI')).toBeInTheDocument();
    // Should include the textual labels 'exists'/'missing'
    expect(screen.getByTestId('artifacts-ODI').textContent).toMatch(/exists/i);
    expect(screen.getByTestId('artifacts-ODI').textContent).toMatch(/missing/i);
  });
});
