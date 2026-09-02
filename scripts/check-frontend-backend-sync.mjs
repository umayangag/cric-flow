#!/usr/bin/env node
/**
 * Check that go-app and ml-service expose canonical config used by the frontend.
 * Frontend fetches formats from GET /api/canonical/formats and model metadata from GET /api/ml/model-metadata.
 * This script validates the backend sources only (no frontend constants to compare).
 * Run from repo root. Used by: make frontend-backend-sync-check, check-all, and CI.
 */

import { spawnSync } from 'child_process';
import * as path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(__dirname, '..');

const EXPECTED_FORMATS = ['TEST', 'ODI', 'T20', 'T20I'];
// The model-metadata card the Workbench renders. One family is left: the windowed-form win
// classifier, which P-6 removes. The XI models describe themselves through GET /xi/status and
// L4's report, not through this endpoint.
const EXPECTED_MODEL_KEYS = ['win'];

function run(cwd, cmd, args, env = {}) {
  const r = spawnSync(cmd, args, {
    cwd,
    env: { ...process.env, ...env },
    encoding: 'utf8',
    maxBuffer: 4 * 1024 * 1024,
  });
  if (r.error) throw r.error;
  return { stdout: r.stdout || '', stderr: r.stderr || '', status: r.status };
}

function main() {
  // 1) go-app canonical formats
  const goResult = run(path.join(ROOT, 'go-app'), 'go', ['run', './cmd/print_canonical']);
  if (goResult.status !== 0) {
    console.error('[check-frontend-backend-sync] go-app print_canonical failed:', goResult.stderr || goResult.stdout);
    process.exit(1);
  }
  let goOut;
  try {
    goOut = JSON.parse(goResult.stdout.trim());
  } catch (e) {
    console.error('[check-frontend-backend-sync] Failed to parse go-app JSON:', e.message);
    process.exit(1);
  }
  const formats = goOut.formats;
  if (!Array.isArray(formats) || formats.length !== EXPECTED_FORMATS.length) {
    console.error('[check-frontend-backend-sync] go-app formats must be array of length', EXPECTED_FORMATS.length, 'got', formats);
    process.exit(1);
  }
  const formatSet = new Set(formats);
  for (const f of EXPECTED_FORMATS) {
    if (!formatSet.has(f)) {
      console.error('[check-frontend-backend-sync] go-app formats missing expected code:', f, 'got', formats);
      process.exit(1);
    }
  }

  // 2) ml-service model metadata (expected keys present with features/outputs)
  const mlCode = `
import json
from app.model_metadata import get_model_metadata
print(json.dumps(get_model_metadata()))
`;
  const mlResult = run(path.join(ROOT, 'ml-service'), 'python', ['-c', mlCode], { PYTHONPATH: '.' });
  if (mlResult.status !== 0) {
    console.error('[check-frontend-backend-sync] ml-service model_metadata failed:', mlResult.stderr || mlResult.stdout);
    process.exit(1);
  }
  let mlOut;
  try {
    mlOut = JSON.parse(mlResult.stdout.trim());
  } catch (e) {
    console.error('[check-frontend-backend-sync] Failed to parse ml-service JSON:', e.message);
    process.exit(1);
  }
  for (const key of EXPECTED_MODEL_KEYS) {
    if (!mlOut[key] || !Array.isArray(mlOut[key].features) || !Array.isArray(mlOut[key].outputs)) {
      console.error('[check-frontend-backend-sync] ml-service model_metadata missing or invalid key:', key);
      process.exit(1);
    }
  }
  const extra = Object.keys(mlOut).filter((k) => !EXPECTED_MODEL_KEYS.includes(k));
  if (extra.length) {
    console.error('[check-frontend-backend-sync] ml-service model_metadata has unexpected keys:', extra);
    process.exit(1);
  }

  console.log('[check-frontend-backend-sync] Backend canonical formats and model metadata OK.');
}

main();
