import { describe, it, expect } from 'vitest';
import { systemMap } from './contract';
import {
  emptySources,
  formatValue,
  MISSING,
  resolveBinding,
  resolveGates,
  resolvePath,
} from './bindings';
import type { BindingSources } from './bindings';
import type { EvaluationReport, OpsStatusDTO, XiStatusResponse } from '../types';

/**
 * The other half of "no number is written into the map": the paths have to be right.
 *
 * A mistyped path is the one way this design fails quietly — the value reads as a dash and
 * looks like an endpoint that had nothing to say. So the fixtures below are typed as the
 * response types the app already declares, and every binding in the contract is resolved
 * against them and required to find something. Rename a field in `types.ts` and the
 * fixture stops compiling; change a path in the contract and this stops passing.
 */

const foldStat = (mean: number) => ({ mean, sd: 0.01, n_folds: 4 });

const opsStatus: OpsStatusDTO = {
  timestamp: '2026-09-02T10:00:00Z',
  services: { api_health: true, api_readiness: true, ml_health: true },
  dataset: { path: '/data/cricsheet', match_files: 19345, bytes: 1073741824 },
  db: {
    connected: true,
    counts: { matches: 19345, players: 14210 },
    migration: { status: 'ok', current: 8, expected: 8 },
    last_match_import_at: '2026-09-01T22:14:03Z',
    table_stats: [
      { table_name: 'ball_event', row_count: 11500000 },
      { table_name: 'match', row_count: 19345 },
    ],
  },
  artifacts: {
    current_run: '20260901T120000Z-abc1234',
    loaded_run: '20260901T120000Z-abc1234',
  },
  // The map's ratings bindings read the one freshness object, like every other surface
  // (P2-1) — not ml-service's verdict copied inside the artifacts section.
  freshness: {
    served: {
      status: 'fresh',
      fresh: true,
      data_age_days: 1,
      max_age_days: 14,
      data_through: '2026-09-02',
      ratings_through: '2026-09-01',
      code: null,
    },
    database: { T20: { latest_match_date: '2026-09-01', age_days: 1, match_count: 11724 } },
    retrain_due: {
      status: 'up_to_date',
      days_behind: 0,
      latest_match_date: '2026-09-01',
      format: 'T20',
    },
  },
};

const xiStatus: XiStatusResponse = {
  loaded: true,
  formats: ['T20', 'T20I', 'ODI', 'TEST'],
  performance_formats: ['T20', 'T20I', 'ODI'],
  players: 14210,
  ratings_through: '2026-09-01',
  run_id: '20260901T120000Z-abc1234',
  error: 'none',
  manifest: {
    run_id: '20260901T120000Z-abc1234',
    created_at: '2026-09-01T12:00:00Z',
    cutoff: '2025-09-01',
    dataset_sha: 'd41d8cd98f00b204e9800998ecf8427e',
    git_sha: '8352816abcdef',
    formats: ['T20', 'T20I', 'ODI', 'TEST'],
  },
};

const formatReport: EvaluationReport['formats'][string] = {
  n_matches: 4210,
  walk_forward: {
    folds: [],
    summary: {
      objective_auc: foldStat(0.702),
      objective_brier: foldStat(0.213),
      display_auc: foldStat(0.724),
      base_rate_brier: foldStat(0.249),
      swap_violation_share: foldStat(0.003),
      display_swap_violation_share: foldStat(0.048),
      specific_vs_typical_delta: foldStat(0.012),
      performance: {
        targets: {
          runs: {
            headline: true,
            model: {
              within_match_spearman: foldStat(0.331),
              interval: { coverage_80: foldStat(0.802), width_80: foldStat(41.6) },
            },
          },
        },
      },
    },
  },
  locked: { cutoff: '2025-09-01', end: '2026-09-01', n_train: 3800, n_eval: 410 },
  simulation_decision: {
    simulated_win_probability_within_tolerance: true,
    shared_factor: true,
    served: false,
  },
  selection_decision: {
    agreement: 0.589,
    bar: 0.51,
    passes_derived_bar: true,
    optimised_selection_served: true,
    reason: 'E5 lineup-only clears its derived bar',
  },
};

const report: EvaluationReport = {
  generated_at: '2026-09-01T18:20:00Z',
  source: 'ml.xi.evaluate',
  cutoffs: ['2024-09-01', '2025-03-01'],
  locked_start: '2026-09-02',
  locked_window: {
    start: '2026-09-02',
    rotated_on: '2026-09-02',
    previous_start: '2025-09-01',
    reason: 'the migration read the previous window',
    retired_into_folds: ['2025-09-01'],
  },
  n_rows: 19345,
  n_player_rows: 425590,
  formats: { T20I: formatReport },
  serving_parity: { passed: true, max_abs_difference: 0, matches_compared: 50 },
  gates: {
    registry: {
      'H-17': {
        id: 'H-17',
        name: 'Objective ranks (format scope)',
        varies: "the two elevens' as-of features",
        fixed: "the fold's cutoff",
        decides: 'walk-forward mean objective AUC >= 0.65',
        report_path: 'walk_forward.summary.objective_auc',
      },
      'H-8': {
        id: 'H-8',
        name: 'Train / serve parity',
        varies: 'the code path',
        fixed: 'the last 50 matches',
        decides: 'max abs difference <= 1e-9',
        report_path: 'report:serving_parity.passed',
      },
    },
    passed: true,
  },
};

const sources: BindingSources = { ops_status: opsStatus, xi_status: xiStatus, report };

describe('resolving live values', () => {
  it('finds every path the contract declares, for every node', () => {
    const missing: string[] = [];
    for (const node of systemMap.nodes) {
      for (const binding of node.bindings ?? []) {
        for (const value of resolveBinding(binding, sources)) {
          if (!value.present) missing.push(`${node.id}.${binding.key} (${binding.path})`);
        }
      }
    }
    expect(missing).toEqual([]);
  });

  it('renders a dash rather than breaking when an endpoint carries nothing', () => {
    for (const node of systemMap.nodes) {
      for (const binding of node.bindings ?? []) {
        for (const value of resolveBinding(binding, emptySources)) {
          expect(value.text, `${node.id}.${binding.key}`).toBe(MISSING);
          expect(value.present).toBe(false);
        }
      }
    }
  });

  it('reads a per-format value once for each format the report holds', () => {
    const binding = {
      key: 'objective_auc',
      label: 'Objective AUC',
      source: 'report' as const,
      path: 'formats.{format}.walk_forward.summary.objective_auc',
      value_kind: 'ratio3' as const,
      per_format: true,
    };
    const values = resolveBinding(binding, sources);
    expect(values.map((value) => value.format)).toEqual(['T20I']);
    expect(values[0].text).toBe('0.702');
  });

  it('picks a row out of a list by naming the field, not the index', () => {
    expect(resolvePath(opsStatus, 'db.table_stats[table_name=ball_event].row_count')).toBe(
      11500000,
    );
    expect(resolvePath(opsStatus, 'db.table_stats[table_name=nope].row_count')).toBeUndefined();
  });

  it('reads a walk-forward mean and a bare number as the same measurement', () => {
    expect(formatValue({ mean: 0.702, sd: 0.01, n_folds: 4 }, 'ratio3')).toBe('0.702');
    expect(formatValue(0.702, 'ratio3')).toBe('0.702');
  });
});

describe('the gates, read from the report', () => {
  it('takes each gate its name, its terms and the path to its own number', () => {
    const gates = resolveGates(['H-17', 'H-8'], report);
    expect(gates[0].name).toBe('Objective ranks (format scope)');
    expect(gates[0].values).toEqual([{ format: 'T20I', text: '0.702' }]);
    // A report-scoped gate is one value for the whole run, not one per format, and a
    // gate whose number is a verdict reads as a word rather than as `true`.
    expect(gates[1].values).toEqual([{ format: 'all formats', text: 'yes' }]);
  });

  it('names a gate the report has never heard of rather than dropping it', () => {
    const [gate] = resolveGates(['H-99'], report);
    expect(gate.id).toBe('H-99');
    expect(gate.decides).toBe(MISSING);
    expect(gate.values).toEqual([]);
  });
});
