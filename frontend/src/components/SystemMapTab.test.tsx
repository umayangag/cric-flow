import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SystemMapTab from './SystemMapTab';
import { systemMap } from '../systemMap/contract';
import { MISSING } from '../systemMap/bindings';
import { MetricGlossaryProvider } from '../context/MetricGlossaryContext';

const mockOpsStatus = vi.fn();
const mockXiStatus = vi.fn();
const mockEvaluationReport = vi.fn();
const mockMetricGlossary = vi.fn();

vi.mock('../api', () => ({
  api: {
    opsStatus: (...args: unknown[]) => mockOpsStatus(...args),
    xiStatus: (...args: unknown[]) => mockXiStatus(...args),
    evaluationReport: (...args: unknown[]) => mockEvaluationReport(...args),
    metricGlossary: (...args: unknown[]) => mockMetricGlossary(...args),
  },
}));

/**
 * The node's own button, not the expand toggle beside it: the card's accessible name is
 * its label followed by its summary, the toggle's is "Expand <label>".
 */
const nodeCardName = (label: string) => (accessibleName: string) =>
  accessibleName.startsWith(label);

const report = {
  generated_at: '2026-09-01T18:20:00Z',
  locked_start: '2025-09-01',
  n_rows: 19345,
  n_player_rows: 425590,
  serving_parity: { passed: true },
  formats: {
    T20I: {
      walk_forward: { summary: { objective_auc: { mean: 0.702, sd: 0.01, n_folds: 4 } } },
      simulation_decision: { simulated_win_probability_within_tolerance: true },
      selection_decision: { optimised_selection_served: true, reason: 'clears its derived bar' },
    },
  },
  gates: {
    passed: true,
    registry: {
      'H-17': {
        id: 'H-17',
        name: 'Objective ranks (format scope)',
        varies: 'the two elevens',
        fixed: "the fold's cutoff",
        decides: 'objective AUC >= 0.65',
        report_path: 'walk_forward.summary.objective_auc',
      },
    },
  },
};

describe('SystemMapTab', () => {
  beforeEach(() => {
    mockOpsStatus.mockReset().mockResolvedValue({ db: { counts: { matches: 19345 } } });
    mockXiStatus.mockReset().mockResolvedValue({ loaded: true, players: 14210, formats: ['T20I'] });
    mockEvaluationReport.mockReset().mockResolvedValue(report);
    mockMetricGlossary.mockReset().mockResolvedValue({ entries: {} });
  });

  it('reads the three endpoints the map binds to', async () => {
    render(<SystemMapTab />);
    await waitFor(() => {
      expect(mockOpsStatus).toHaveBeenCalled();
      expect(mockXiStatus).toHaveBeenCalled();
      expect(mockEvaluationReport).toHaveBeenCalled();
    });
  });

  /**
   * The acceptance clause, asserted rather than eyeballed: every step in the contract is
   * on the map, and every one of them opens a panel that says what it does. A node added
   * to the contract with no summary fails here, not in review.
   */
  it('draws every node in the contract and opens details for each one', async () => {
    const user = userEvent.setup();
    render(<SystemMapTab />);

    for (const node of systemMap.nodes) {
      const box = await screen.findByTestId(`system-map-node-${node.id}`);
      await user.click(within(box).getByRole('button', { name: nodeCardName(node.label) }));

      const detail = screen.getByTestId('system-map-detail');
      expect(within(detail).getByRole('heading', { name: node.label })).toBeInTheDocument();
      expect(within(detail).getByText(node.summary)).toBeInTheDocument();
      expect(within(detail).getByText('Documented in')).toBeInTheDocument();
    }
  });

  it('expands a node with inner structure and lists what is inside it', async () => {
    const user = userEvent.setup();
    render(<SystemMapTab />);

    const ratingPass = await screen.findByTestId('system-map-node-rating-pass');
    await user.click(within(ratingPass).getByRole('button', { name: 'Expand The rating pass' }));
    expect(within(ratingPass).getByText('What a player is worth')).toBeInTheDocument();

    await user.click(within(ratingPass).getByRole('button', { name: 'Collapse The rating pass' }));
    expect(within(ratingPass).queryByText('What a player is worth')).not.toBeInTheDocument();
  });

  it('shows a gate with the terms and the number the report gives it', async () => {
    const user = userEvent.setup();
    render(<SystemMapTab />);

    const harness = await screen.findByTestId('system-map-node-harness');
    await user.click(
      within(harness).getByRole('button', { name: nodeCardName('The evaluation harness') }),
    );

    const detail = screen.getByTestId('system-map-detail');
    await waitFor(() => {
      expect(within(detail).getByText(/Objective ranks \(format scope\)/)).toBeInTheDocument();
    });
    expect(within(detail).getByText('T20I: 0.702')).toBeInTheDocument();
  });

  /**
   * The map has to survive the ordinary state of a fresh box: nothing imported, no run
   * loaded and `make evaluate` never run. Every value is missing and the tab still draws.
   */
  it('degrades a live value the endpoints do not carry to a dash', async () => {
    mockOpsStatus.mockResolvedValue({});
    mockXiStatus.mockResolvedValue({});
    mockEvaluationReport.mockResolvedValue({});
    const user = userEvent.setup();
    render(<SystemMapTab />);

    const eventStore = await screen.findByTestId('system-map-node-event-store');
    await user.click(within(eventStore).getByRole('button', { name: nodeCardName('Event store') }));

    const detail = screen.getByTestId('system-map-detail');
    await waitFor(() => expect(within(detail).getByText('Matches')).toBeInTheDocument());
    expect(within(detail).getAllByText(MISSING).length).toBeGreaterThan(0);
  });

  /**
   * The map holds no metric prose of its own: a binding names a glossary key and L-1's
   * explainer supplies the words. This is the seam — that the key reaches `MetricInfo`,
   * and that a metric the glossary has never heard of simply has no explainer rather
   * than an empty one.
   */
  it('hands a metric key to L-1s explainer and shows what the glossary says', async () => {
    mockMetricGlossary.mockResolvedValue({
      entries: {
        objective_auc: {
          key: 'objective_auc',
          name: 'Objective AUC',
          explanation: 'How often the model ranks the actual winner higher.',
          band: '0.70-0.75 is this system’s measured range.',
          better: 'higher is better',
        },
      },
    });
    const user = userEvent.setup();
    render(
      <MetricGlossaryProvider>
        <SystemMapTab />
      </MetricGlossaryProvider>,
    );

    const objective = await screen.findByTestId('system-map-node-win-objective');
    await user.click(
      within(objective).getByRole('button', { name: nodeCardName('Win model — objective') }),
    );

    const detail = screen.getByTestId('system-map-detail');
    const explainer = await within(detail).findByRole('button', {
      name: /Objective AUC \(walk-forward mean\)/,
    });
    await user.click(explainer);
    expect(
      screen.getByText('How often the model ranks the actual winner higher.'),
    ).toBeInTheDocument();

    // `swap_violation_share` is in the contract too; the stub glossary does not carry it,
    // so it gets no explainer and no empty popover.
    expect(
      within(detail).queryByRole('button', { name: /Upgrades that lower/ }),
    ).not.toBeInTheDocument();
  });

  it('reports a failed call without taking the map down with it', async () => {
    mockEvaluationReport.mockRejectedValue(new Error('report unavailable'));
    render(<SystemMapTab />);

    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument());
    expect(await screen.findByTestId('system-map-node-event-store')).toBeInTheDocument();
  });
});
