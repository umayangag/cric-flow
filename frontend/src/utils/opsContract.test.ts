import { describe, it, expect } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { derivePipelineSteps, type PipelineStepId } from './pipelineSteps';
import {
  FORECAST_SOURCES,
  FRESHNESS_STATUSES,
  POOL_EXCLUSION_REASONS,
  POOL_SOURCES,
  RATINGS_STALE_CODE,
  PREDICTION_STATES,
  RETRAIN_STATUSES,
  SELECTION_OBJECTIVES,
  SELECTION_ROLES,
  SIMULATOR_POPULATIONS,
  TEAM_GENDERS,
  TRACK_RECORD_METRIC_KEYS,
  WIN_PROBABILITY_SOURCES,
} from '../types';

/**
 * The backend/frontend contract test.
 *
 * contracts/ops-console.contract.json is generated from go-app's pipeline step
 * registry (see registry_test.go). Both sides assert against it, so the UI cannot
 * offer a step the backend refuses, hide one it accepts, or keep sending a parameter
 * the backend has retired — the class of bug that produced both the missing
 * train_combination_meta step and the use_unified_model toggle that only ever 400'd.
 *
 * When this fails: regenerate the contract from Go, then make the UI match it.
 */

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, '../../..');
const frontendSrc = resolve(here, '..');

type ContractStep = {
  id: PipelineStepId;
  command: string;
  label: string;
  optional: boolean;
  requires: string[];
  prerequisite?: string;
};

type Contract = {
  version: number;
  pipeline_steps: ContractStep[];
  rejected_query_params: string[];
  rejected_body_params: string[];
  /** The training cutoff's wire format: what go-app sends and ml-service parses (H-24). */
  cutoff: { pattern: string; hint: string; example: string };
  /** The gender half of a team's identity, as all three components spell it (H-24, D-10). */
  team_genders: string[];
  /** How a candidate pool was chosen, and why the ledger excluded a player (H-24, D-12). */
  pool_sources: string[];
  pool_exclusion_reasons: string[];
  /** The constraint state a "why this player" card may name (H-24, P1-3). */
  selection_roles: string[];
  /** The model behind the headline probability, and behind the per-player numbers (H-24, P1-4). */
  win_probability_sources: string[];
  forecast_sources: string[];
  /** How an eleven was arrived at, as the prediction record stores it (H-24, P2-3). */
  selection_objectives: string[];
  /** The one freshness vocabulary, and the code a refused prediction carries (H-24, P2-1). */
  freshness_statuses: string[];
  retrain_statuses: string[];
  ratings_stale_code: string;
  /** The track record's states and simulator populations, and its L-1 keys (H-24, P2-4). */
  prediction_states: string[];
  simulator_populations: string[];
  track_record_metric_keys: string[];
};

const contract: Contract = JSON.parse(
  readFileSync(join(repoRoot, 'contracts', 'ops-console.contract.json'), 'utf8'),
) as Contract;

/**
 * Every production .ts/.tsx file under frontend/src. Tests are excluded: a test that
 * asserts a parameter is *not* sent has to name it, and that is not a violation.
 */
function productionSources(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) return productionSources(full);
    if (!/\.tsx?$/.test(entry)) return [];
    if (/\.test\.tsx?$/.test(entry)) return [];
    return [full];
  });
}

describe('ops console contract', () => {
  const uiStepIds = derivePipelineSteps(null).map((s) => s.id);

  it('offers exactly the steps the backend accepts, in the backend order', () => {
    expect(uiStepIds).toEqual(contract.pipeline_steps.map((s) => s.id));
  });

  it('marks the same steps optional as the backend does', () => {
    const uiSteps = derivePipelineSteps(null);
    for (const step of contract.pipeline_steps) {
      if (!step.optional) continue;
      const ui = uiSteps.find((s) => s.id === step.id);
      expect(ui?.status, `${step.id} should be presented as optional`).toBe('optional');
    }
  });

  it('states the prerequisite for every step the backend says has one', () => {
    const uiSteps = derivePipelineSteps(null);
    for (const step of contract.pipeline_steps) {
      if (!step.prerequisite) continue;
      const ui = uiSteps.find((s) => s.id === step.id);
      expect(
        ui?.prerequisite,
        `${step.id} must tell the operator about its precondition`,
      ).toBeTruthy();
    }
  });

  it('records the same data_migrations command the backend writes', () => {
    // Without this the UI can only recognise its own run history by hand-typed
    // strings — which is how the six step tables drifted before F-1 replaced them.
    const uiSteps = derivePipelineSteps(null);
    for (const step of contract.pipeline_steps) {
      const ui = uiSteps.find((s) => s.id === step.id);
      expect(ui?.migrationCommand, `${step.id} migration command`).toBe(step.command);
    }
  });

  it('gives every step a label, command and description', () => {
    for (const step of derivePipelineSteps(null)) {
      expect(step.label, `${step.id} label`).toBeTruthy();
      expect(step.command, `${step.id} command`).toBeTruthy();
      expect(step.description, `${step.id} description`).toBeTruthy();
    }
  });

  it('never constructs a query parameter the backend rejects', () => {
    const offenders: string[] = [];
    for (const file of productionSources(frontendSrc)) {
      const text = readFileSync(file, 'utf8');
      for (const param of contract.rejected_query_params) {
        if (text.includes(param)) {
          offenders.push(`${file.slice(repoRoot.length + 1)} mentions "${param}"`);
        }
      }
    }
    expect(offenders).toEqual([]);
  });

  /**
   * The console's cutoff box is where an operator types this value, so the format it
   * asks for is the third side of the same contract (H-24). D-9 was the two services
   * disagreeing about it; a dialog that asked for a different one would be the same
   * defect with a person in the middle.
   */
  it('asks the operator for the cutoff format the services agreed on', () => {
    const dialog = readFileSync(join(frontendSrc, 'components', 'PipelineStepDialog.tsx'), 'utf8');

    expect(dialog).toContain(`Cutoff (${contract.cutoff.hint})`);
    expect(contract.cutoff.example).toMatch(new RegExp(contract.cutoff.pattern));
  });

  /**
   * Body fields are checked against api.ts alone, not every source.
   *
   * Their names are ordinary words — "weather" is also a section of /ops/status — so a
   * repo-wide grep would refuse the UI for rendering a response. api.ts is the only
   * place a request body is built, and the request types there are closed object
   * literals, so TypeScript refuses an extra key at every call site.
   */
  it('never constructs a body field the backend rejects', () => {
    const apiModule = readFileSync(join(frontendSrc, 'api.ts'), 'utf8');
    const offenders = contract.rejected_body_params.filter((param) => apiModule.includes(param));
    expect(offenders).toEqual([]);
  });

  /**
   * The UI's gender vocabulary is the contract's, not a copy of it.
   *
   * D-10 put gender on the request wire; ml-service already matched on the literal to group
   * E7's context baselines. Three components agreeing on one word by hand-typing it three
   * times is the shape D-9 had, so this is the third side's assertion.
   */
  it('spells the team genders the way the backend does', () => {
    expect([...TEAM_GENDERS].sort()).toEqual([...contract.team_genders].sort());
  });

  /**
   * The candidate pool's vocabulary is the contract's too (H-24, D-12).
   *
   * go-app names the source of every pool it builds and the reason for every player the
   * retirement ledger removes; the Upcoming-match tab renders both. A source the UI does
   * not recognise would be shown as nothing while looking perfectly fine on the wire —
   * which is exactly the silence D-12 fixed, put back one layer up.
   */
  it('spells the pool sources and exclusion reasons the way the backend does', () => {
    expect([...POOL_SOURCES].sort()).toEqual([...contract.pool_sources].sort());
    expect([...POOL_EXCLUSION_REASONS].sort()).toEqual([...contract.pool_exclusion_reasons].sort());
  });

  /**
   * The selection roles are the contract's too (H-24, P1-3).
   *
   * ml-service computes them from the constraint predicates its optimiser evaluates and
   * go-app carries them onto the prediction; the "why this player" card turns each into a
   * chip. A role the UI does not recognise would be a constraint the objective really did
   * read and the card silently dropped.
   */
  it('spells the selection roles the way the backend does', () => {
    expect([...SELECTION_ROLES].sort()).toEqual([...contract.selection_roles].sort());
  });

  /**
   * The two source vocabularies are the contract's too (H-24, P1-4).
   *
   * The Lab names the model behind the headline probability and behind the per-player
   * numbers, and opens each name's explainer from the glossary. A source the UI cannot spell
   * would be a number on screen with no model named behind it.
   */
  it('spells the win-probability and forecast sources the way the backend does', () => {
    expect([...WIN_PROBABILITY_SOURCES].sort()).toEqual(
      [...contract.win_probability_sources].sort(),
    );
    expect([...FORECAST_SOURCES].sort()).toEqual([...contract.forecast_sources].sort());
  });

  /**
   * The selection objectives are the contract's too (H-24, P2-3).
   *
   * The Lab titles its XI tables off the objective, and the prediction record stores it as
   * a column and lists it. `fixed` is what says a stored answer is an eleven the user built
   * — a scenario the track record lists and never scores — so a side spelling it
   * differently would score a hypothetical eleven as a forecast about a real fixture.
   */
  it('spells the selection objectives the way the backend does', () => {
    expect([...SELECTION_OBJECTIVES].sort()).toEqual([...contract.selection_objectives].sort());
  });

  /**
   * The freshness vocabulary is the contract's too (H-24, P2-1).
   *
   * There is one verdict in this system — H-11's — and the console renders it as a badge,
   * the Lab's readiness notice as a warning, and `ErrorNotice` keys its remedy off the
   * refusal's code. A status word or a code the UI spells differently is a verdict shown
   * as `unknown` over a service that answered perfectly clearly, which is the shape of the
   * disagreement P2-1 closed.
   */
  it('spells the freshness verdict the way the backend does', () => {
    expect([...FRESHNESS_STATUSES].sort()).toEqual([...contract.freshness_statuses].sort());
    expect([...RETRAIN_STATUSES].sort()).toEqual([...contract.retrain_statuses].sort());
    expect(RATINGS_STALE_CODE).toEqual(contract.ratings_stale_code);
  });

  /**
   * L-1's completeness gate, from the UI's side (P1-3 clause 4).
   *
   * `MetricInfo` renders *nothing at all* for a key the served glossary does not carry, so
   * a mistyped or invented key is a labelled number with no explainer and no error — the
   * silent failure the explainers exist to prevent. Every `metricKey` a production
   * component names must therefore have an entry in `ml-service/ml/xi/glossary.py`, which
   * is the one place that prose lives (H-24: the frontend holds no metric copy of its own).
   */
  it('names only metric keys the service glossary can explain', () => {
    const glossary = readFileSync(join(repoRoot, 'ml-service', 'ml', 'xi', 'glossary.py'), 'utf8');
    const explained = new Set(
      [...glossary.matchAll(/\bkey="([a-z0-9_]+)"/g)].map((match) => match[1]),
    );
    expect(explained.size).toBeGreaterThan(20);

    const missing: string[] = [];
    for (const file of productionSources(frontendSrc)) {
      const source = readFileSync(file, 'utf8');
      for (const match of source.matchAll(/metricKey="([a-z0-9_]+)"/g)) {
        if (!explained.has(match[1])) missing.push(`${file}: ${match[1]}`);
      }
    }
    expect(missing, 'every labelled number needs an L-1 entry').toEqual([]);
  });

  /**
   * A side's label ("India (men)") comes from the backend as `display_name`, so the UI never
   * builds one from a gender literal. If it did, the picker and the response's echo of what
   * was scored could disagree about the same team.
   */
  it('never builds a side label out of a gender literal', () => {
    const offenders: string[] = [];
    for (const file of productionSources(frontendSrc)) {
      if (file.endsWith(join('src', 'types.ts'))) continue; // where the vocabulary is declared
      const text = readFileSync(file, 'utf8');
      for (const gender of contract.team_genders) {
        if (text.includes(`'${gender}'`) || text.includes(`"${gender}"`)) {
          offenders.push(`${file.slice(repoRoot.length + 1)} spells out "${gender}"`);
        }
      }
    }
    expect(offenders).toEqual([]);
  });

  it("spells the track record's states and simulator populations the way the backend does", () => {
    expect([...PREDICTION_STATES]).toEqual(contract.prediction_states);
    expect([...SIMULATOR_POPULATIONS]).toEqual(contract.simulator_populations);
  });

  it("labels the track record's numbers under exactly the glossary keys the contract declares", () => {
    // ml-service's completeness gate asserts every key here has a glossary entry; this
    // side asserts the tab renders no key outside the list, so a number cannot reach the
    // surface without an explainer behind it (L-1).
    expect([...TRACK_RECORD_METRIC_KEYS]).toEqual(contract.track_record_metric_keys);
    const tabSources = productionSources(join(frontendSrc, 'components')).filter((file) =>
      file.includes('TrackRecord'),
    );
    expect(tabSources.length).toBeGreaterThan(0);
    const used = new Set<string>();
    for (const file of tabSources) {
      for (const match of readFileSync(file, 'utf8').matchAll(/metricKey="([a-z0-9_]+)"/g)) {
        used.add(match[1]);
      }
    }
    expect([...used].sort()).toEqual([...contract.track_record_metric_keys].sort());
  });
});
