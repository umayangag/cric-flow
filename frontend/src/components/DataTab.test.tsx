import { render, screen, waitFor, cleanup, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import React from 'react';
import DataTab from './DataTab';

const mockFeeds = vi.fn();
const mockStaged = vi.fn();
const mockDatasets = vi.fn();
const mockStatus = vi.fn();
const mockFetch = vi.fn();
const mockExtract = vi.fn();

vi.mock('../api', () => ({
  api: {
    opsDataFeeds: (...a: unknown[]) => mockFeeds(...a),
    opsDataStaged: (...a: unknown[]) => mockStaged(...a),
    opsDatasets: (...a: unknown[]) => mockDatasets(...a),
    opsStatus: (...a: unknown[]) => mockStatus(...a),
    opsDataFetch: (...a: unknown[]) => mockFetch(...a),
    opsDataExtract: (...a: unknown[]) => mockExtract(...a),
    opsPipelineStop: vi.fn(),
    subscribePipelineProgress: vi.fn(() => new Promise(() => {})),
  },
}));

const FEEDS = {
  feeds: [
    {
      id: 'all',
      label: 'All matches',
      url: 'https://cricsheet.org/downloads/all_json.zip',
      description: 'Everything',
    },
    {
      id: 't20s',
      label: "Men's T20 internationals",
      url: 'https://cricsheet.org/downloads/t20s_json.zip',
      description: 'T20Is',
    },
  ],
  allowed_hosts: ['cricsheet.org', 'www.cricsheet.org'],
  staging_dir: '/data/cricsheet/_staging',
};

const STAGED = {
  staging_dir: '/data/cricsheet/_staging',
  dataset_dir: '/data/cricsheet',
  archives: [
    {
      filename: 'all_json.zip',
      bytes: 1048576,
      modified: '2026-08-26T10:00:00Z',
      sha256: 'abcdef0123456789abcdef',
      source_url: 'https://cricsheet.org/downloads/all_json.zip',
      feed_id: 'all',
    },
  ],
};

const REGISTRY = {
  dataset_dir: '/data/cricsheet',
  live_sha256: 'abcdef0123456789abcdef',
  datasets: [
    {
      id: 1,
      sha256: 'abcdef0123456789abcdef',
      feed: 'all',
      source_url: 'https://cricsheet.org/downloads/all_json.zip',
      filename: 'all_json.zip',
      bytes: 1048576,
      fetched_at: '2026-08-26T10:00:00Z',
      extracted_at: '2026-08-26T10:05:00Z',
      entry_count: 20000,
      match_files: 19998,
      created_at: '2026-08-26T10:00:00Z',
      updated_at: '2026-08-26T10:05:00Z',
      live: true,
    },
  ],
};

function primeHappyPath(overrides?: { status?: unknown; registry?: unknown; staged?: unknown }) {
  mockFeeds.mockResolvedValue(FEEDS);
  mockStaged.mockResolvedValue(overrides?.staged ?? STAGED);
  mockDatasets.mockResolvedValue(overrides?.registry ?? REGISTRY);
  mockStatus.mockResolvedValue(
    overrides?.status ?? {
      pipeline: { steps: { fetch: { running: false }, extract: { running: false } } },
    },
  );
}

describe('DataTab', () => {
  beforeEach(() => {
    for (const m of [mockFeeds, mockStaged, mockDatasets, mockStatus, mockFetch, mockExtract]) {
      m.mockReset();
    }
  });
  afterEach(cleanup);

  it('states the allowlist before a URL is typed, not after it is refused', async () => {
    primeHappyPath();
    render(<DataTab />);

    await waitFor(() => expect(screen.getByText(/only download from/i)).toBeInTheDocument());
    expect(screen.getByText(/cricsheet\.org, www\.cricsheet\.org/)).toBeInTheDocument();
  });

  it('sends the chosen feed and never a feed and url together', async () => {
    primeHappyPath();
    mockFetch.mockResolvedValue({
      status: 202,
      data: { status: 'started', filename: 'all_json.zip' },
    });
    render(<DataTab />);

    await waitFor(() => expect(screen.getByLabelText('Feed')).toBeInTheDocument());
    await userEvent.click(screen.getByLabelText('Feed'));
    await userEvent.click(await screen.findByRole('option', { name: 'All matches' }));
    await userEvent.click(screen.getByRole('button', { name: 'Fetch' }));

    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith({ feed: 'all' }));
  });

  it('surfaces a refused source together with what would be accepted', async () => {
    primeHappyPath();
    mockFetch.mockResolvedValue({
      status: 400,
      data: {
        error: 'host is not on the Cricsheet allowlist: evil.example',
        allowed_hosts: ['cricsheet.org'],
      },
    });
    render(<DataTab />);

    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Explicit URL' })).toBeInTheDocument(),
    );
    await userEvent.click(screen.getByRole('button', { name: 'Explicit URL' }));
    await userEvent.type(screen.getByLabelText('Archive URL'), 'https://evil.example/x.zip');
    await userEvent.click(screen.getByRole('button', { name: 'Fetch' }));

    const alert = await screen.findByText(/not on the Cricsheet allowlist/i);
    expect(alert).toBeInTheDocument();
    expect(screen.getByText(/Allowed hosts: cricsheet\.org/)).toBeInTheDocument();
  });

  it('warns that extracting replaces the dataset directory before the click', async () => {
    primeHappyPath();
    render(<DataTab />);

    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Extract' })).toBeInTheDocument(),
    );
    expect(screen.getByText(/replaces/i)).toBeInTheDocument();
    expect(screen.getByText(/previous contents are kept/i)).toBeInTheDocument();
  });

  it('extracts the selected archive', async () => {
    primeHappyPath();
    mockExtract.mockResolvedValue({
      status: 202,
      data: { status: 'started', archive: 'all_json.zip' },
    });
    render(<DataTab />);

    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Extract' })).toBeInTheDocument(),
    );
    await userEvent.click(screen.getByRole('button', { name: 'Extract' }));

    await waitFor(() => expect(mockExtract).toHaveBeenCalledWith({ archive: 'all_json.zip' }));
  });

  it('says what to do when nothing is staged instead of offering a dead button', async () => {
    primeHappyPath({
      staged: {
        staging_dir: '/data/cricsheet/_staging',
        dataset_dir: '/data/cricsheet',
        archives: [],
      },
    });
    render(<DataTab />);

    await waitFor(() => expect(screen.getByText(/Nothing staged/i)).toBeInTheDocument());
    expect(screen.queryByRole('button', { name: 'Extract' })).not.toBeInTheDocument();
  });

  it('marks the live dataset in the registry', async () => {
    primeHappyPath();
    render(<DataTab />);

    const row = await screen.findByText('all_json.zip', { selector: 'td' });
    expect(within(row).getByText('live')).toBeInTheDocument();
  });

  // The registry derives "live" from the dataset directory's manifest, so a directory
  // populated some other way yields a live digest that matches no row. Rendering that
  // as an unremarkable table would read as "nothing is live", which is a different
  // and wrong answer.
  it('calls out a live dataset the registry has never seen', async () => {
    primeHappyPath({
      registry: { dataset_dir: '/data/cricsheet', live_sha256: 'unknown-digest', datasets: [] },
    });
    render(<DataTab />);

    await waitFor(() => expect(screen.getByText(/registry has never seen/i)).toBeInTheDocument());
  });

  it('disables both actions while a dataset step is already running', async () => {
    primeHappyPath({
      status: { pipeline: { steps: { fetch: { running: true }, extract: { running: false } } } },
    });
    render(<DataTab />);

    await waitFor(() => expect(screen.getByRole('button', { name: 'Fetch' })).toBeDisabled());
    expect(screen.getByRole('button', { name: 'Extract' })).toBeDisabled();
    expect(screen.getAllByText(/already running/i).length).toBeGreaterThan(0);
  });

  it('reports a failure to load rather than rendering an empty tab', async () => {
    mockFeeds.mockRejectedValue(new Error('go-app unreachable'));
    mockStaged.mockRejectedValue(new Error('go-app unreachable'));
    mockDatasets.mockRejectedValue(new Error('go-app unreachable'));
    mockStatus.mockRejectedValue(new Error('go-app unreachable'));
    render(<DataTab />);

    await waitFor(() => expect(screen.getByText(/go-app unreachable/i)).toBeInTheDocument());
  });
});
