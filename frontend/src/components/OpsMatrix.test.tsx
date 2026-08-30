import { render, screen } from '@testing-library/react';
import React from 'react';
import OpsMatrix from './OpsMatrix';
import { ARTIFACT_KINDS } from '../utils/artifactKinds';

const CANONICAL_FORMATS = ['TEST', 'ODI', 'T20I', 'T20'];

describe('OpsMatrix', () => {
  it('explains why the formats are missing when the last run did not finish', () => {
    const data = {
      formats: { T20: { status: 'missing' }, ODI: { status: 'missing' } },
      last_error: 'upsert raw stats venue pid=61: context canceled',
    };

    render(
      <OpsMatrix type="precompute" title="Precompute" data={data} formats={CANONICAL_FORMATS} />,
    );

    expect(
      screen.getByText(/Last run did not finish: upsert raw stats venue pid=61/),
    ).toBeInTheDocument();
  });

  it('says nothing about a failure after a clean run', () => {
    const data = { formats: { T20: { status: 'ok' } } };

    render(
      <OpsMatrix type="precompute" title="Precompute" data={data} formats={CANONICAL_FORMATS} />,
    );

    expect(screen.queryByText(/Last run did not finish/)).not.toBeInTheDocument();
  });

  it('renders precompute statuses per format', () => {
    const data = {
      formats: {
        TEST: { status: 'ok' },
        ODI: { status: 'stale' },
        T20I: { status: 'missing' },
        T20: { status: 'ok' },
      },
    };
    render(
      <OpsMatrix type="precompute" title="Precompute" data={data} formats={CANONICAL_FORMATS} />,
    );
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
    render(
      <OpsMatrix type="artifacts" title="Artifacts" data={data} formats={CANONICAL_FORMATS} />,
    );
    expect(screen.getByTestId('artifacts-ODI')).toBeInTheDocument();
    // Should include the textual labels 'exists'/'missing'
    expect(screen.getByTestId('artifacts-ODI').textContent).toMatch(/exists/i);
    expect(screen.getByTestId('artifacts-ODI').textContent).toMatch(/missing/i);
  });

  it('lists every artifact kind, innings included', () => {
    const data = { formats: { ODI: { innings: { exists: true, loaded: true } } } };

    render(
      <OpsMatrix type="artifacts" title="Artifacts" data={data} formats={CANONICAL_FORMATS} />,
    );

    const cell = screen.getByTestId('artifacts-ODI');
    ARTIFACT_KINDS.forEach((kind) => expect(cell.textContent).toContain(kind));
  });

  it('marks a format stale when a loaded artifact is older than the file on disk', () => {
    const data = {
      formats: {
        ODI: {
          batting: { exists: true, loaded: true },
          bowling: { exists: true, loaded: true },
          fielding: { exists: true, loaded: true },
          extras: { exists: true, loaded: true },
          win: { exists: true, loaded: true },
          innings: { exists: true, loaded: true, stale: true },
        },
      },
    };

    render(
      <OpsMatrix type="artifacts" title="Artifacts" data={data} formats={CANONICAL_FORMATS} />,
    );

    expect(screen.getByTitle('loaded, but an older file than the one on disk')).toBeInTheDocument();
  });
});
