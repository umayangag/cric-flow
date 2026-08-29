import { render, screen, cleanup } from '@testing-library/react';
import React from 'react';
import DatasetRegistryTable from './DatasetRegistryTable';
import type { DatasetRegistryEntry, DatasetRegistryResponse } from '../types';

const ENTRY: DatasetRegistryEntry = {
  id: 1,
  sha256: 'ad589a1127820000000000000000000000000000000000000000000000000000',
  feed: 't20s',
  source_url: 'https://cricsheet.org/downloads/t20s_json.zip',
  filename: 't20s_json.zip',
  bytes: 1024,
  fetched_at: '2026-08-29T06:38:33Z',
  created_at: '2026-08-29T06:38:33Z',
  updated_at: '2026-08-29T06:38:33Z',
  live: false,
};

const registryWith = (
  overrides: Partial<DatasetRegistryResponse> = {},
): DatasetRegistryResponse => ({
  datasets: [ENTRY],
  dataset_dir: '/data/go-app/cricsheet',
  live_sha256: '',
  ...overrides,
});

const NO_MANIFEST = /dataset directory has no manifest/i;
const NEVER_SEEN = /registry has never seen/i;

describe('DatasetRegistryTable', () => {
  afterEach(cleanup);

  it('says why no row is marked live when the directory has no manifest', () => {
    render(<DatasetRegistryTable registry={registryWith()} />);

    expect(screen.getByText(NO_MANIFEST)).toBeTruthy();
    expect(screen.queryByText(NEVER_SEEN)).toBeNull();
    expect(screen.getByText('t20s_json.zip')).toBeTruthy();
  });

  it('reports an unrecognised live dataset when the manifest matches no row', () => {
    render(<DatasetRegistryTable registry={registryWith({ live_sha256: 'deadbeefcafe' })} />);

    expect(screen.getByText(NEVER_SEEN)).toBeTruthy();
    expect(screen.queryByText(NO_MANIFEST)).toBeNull();
  });

  it('warns about neither when a row is marked live', () => {
    render(
      <DatasetRegistryTable
        registry={registryWith({
          datasets: [{ ...ENTRY, live: true }],
          live_sha256: ENTRY.sha256,
        })}
      />,
    );

    expect(screen.queryByText(NO_MANIFEST)).toBeNull();
    expect(screen.queryByText(NEVER_SEEN)).toBeNull();
    expect(screen.getByText('live')).toBeTruthy();
  });

  it('shows the empty-registry message alone when nothing has been acquired', () => {
    render(<DatasetRegistryTable registry={registryWith({ datasets: [] })} />);

    expect(screen.getByText(/No datasets recorded yet/i)).toBeTruthy();
    expect(screen.queryByText(NO_MANIFEST)).toBeNull();
  });

  it('reports the registry as unavailable when it could not be read', () => {
    render(<DatasetRegistryTable />);

    expect(screen.getByText(/Registry unavailable/i)).toBeTruthy();
    expect(screen.queryByText(NO_MANIFEST)).toBeNull();
  });
});
