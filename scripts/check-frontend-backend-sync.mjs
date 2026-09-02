#!/usr/bin/env node
/**
 * Check that go-app and ml-service agree on the two things the frontend depends on and
 * cannot see for itself: the canonical format codes, and the format codes the ML service
 * serves models for.
 *
 * The frontend fetches formats from GET /api/canonical/formats. It used to fetch a
 * model-metadata card too, describing the windowed-form win model's feature order; P-6
 * deleted that model, and the XI models describe themselves through GET /xi/status and
 * L4's report. What replaces the check is the one below: two independent lists of format
 * codes, in two languages, that have to be the same list.
 *
 * Run from repo root. Used by: make frontend-backend-sync-check, check-all, and CI.
 */

import { spawnSync } from 'child_process';
import * as path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(__dirname, '..');

const EXPECTED_FORMATS = ['TEST', 'ODI', 'T20', 'T20I'];

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

  // 2) ml-service's own format codes. They are a separate list in a separate language
  // (ml.config.CANONICAL_FORMAT_CODES), and a run trained for formats go-app cannot name
  // is a model nothing will ever ask for.
  const mlCode = `
import json
from ml.config import get_format_codes
print(json.dumps(get_format_codes()))
`;
  const mlResult = run(path.join(ROOT, 'ml-service'), 'python', ['-c', mlCode], { PYTHONPATH: '.' });
  if (mlResult.status !== 0) {
    console.error('[check-frontend-backend-sync] ml-service format codes failed:', mlResult.stderr || mlResult.stdout);
    process.exit(1);
  }
  let mlFormats;
  try {
    mlFormats = JSON.parse(mlResult.stdout.trim());
  } catch (e) {
    console.error('[check-frontend-backend-sync] Failed to parse ml-service JSON:', e.message);
    process.exit(1);
  }
  if ([...mlFormats].sort().join(',') !== [...formats].sort().join(',')) {
    console.error(
      '[check-frontend-backend-sync] go-app and ml-service disagree on the format codes:',
      formats,
      'vs',
      mlFormats,
    );
    process.exit(1);
  }

  console.log('[check-frontend-backend-sync] go-app and ml-service agree on the format codes.');
}

main();
