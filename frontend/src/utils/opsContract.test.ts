import { describe, it, expect } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { derivePipelineSteps, type PipelineStepId } from './pipelineSteps';

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
});
