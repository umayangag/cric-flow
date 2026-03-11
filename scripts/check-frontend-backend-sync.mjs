#!/usr/bin/env node
/**
 * Check that frontend canonical config (FORMATS, model metadata) aligns with go-app and ml-service.
 * Run from repo root. Fails with non-zero exit and message if any mismatch.
 * Used by: make frontend-backend-sync-check, check-all, and CI.
 */

import { spawnSync } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(__dirname, '..');

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

function compareFormats(goFormats, frontFormats) {
  const goSet = new Set(goFormats);
  const frontSet = new Set(frontFormats);
  if (goSet.size !== frontSet.size || [...goSet].some((f) => !frontSet.has(f))) {
    return {
      ok: false,
      message: `FORMATS mismatch: go-app has [${goFormats.join(', ')}], frontend has [${frontFormats.join(', ')}]. Frontend FORMATS (e.g. in utils/opsStatusHelpers.ts) must match go-app internal/formats CanonicalCodes().`,
    };
  }
  return { ok: true };
}

function compareModelMetadata(mlMeta, frontMeta) {
  const modelKeys = ['batting', 'bowling', 'fielding', 'extras', 'win', 'combination_meta'];
  const frontKeys = frontMeta.MODEL_KEYS || Object.keys(frontMeta.DEFAULT_MODEL_FEATURES || {});
  const frontSet = new Set(frontKeys);
  const mlSet = new Set(Object.keys(mlMeta).filter((k) => modelKeys.includes(k)));
  if (mlSet.size !== frontSet.size || [...mlSet].some((k) => !frontSet.has(k))) {
    return {
      ok: false,
      message: `MODEL_KEYS mismatch: ml-service has [${[...mlSet].sort().join(', ')}], frontend MODEL_KEYS has [${frontKeys.join(', ')}]. Update frontend constants/defaultModelFeatures.ts MODEL_KEYS to match ml-service app/model_metadata.py.`,
    };
  }

  for (const key of modelKeys) {
    if (!mlMeta[key] || !frontMeta.DEFAULT_MODEL_FEATURES?.[key]) continue;
    const m = mlMeta[key];
    const f = frontMeta.DEFAULT_MODEL_FEATURES[key];
    if (JSON.stringify(m.features) !== JSON.stringify(f.features)) {
      return {
        ok: false,
        message: `Model "${key}" features mismatch. ml-service and frontend defaultModelFeatures.ts must list the same features in the same order. Update frontend/src/constants/defaultModelFeatures.ts from ml-service app/model_metadata.py (and feature config).`,
      };
    }
    if (JSON.stringify(m.outputs) !== JSON.stringify(f.outputs)) {
      return {
        ok: false,
        message: `Model "${key}" outputs mismatch. Update frontend defaultModelFeatures.ts outputs to match ml-service app/model_metadata.py.`,
      };
    }
    if (m.level !== f.level) {
      return {
        ok: false,
        message: `Model "${key}" level mismatch: ml has "${m.level}", frontend has "${f.level}".`,
      };
    }
    if (Boolean(m.hasScaler) !== Boolean(f.hasScaler)) {
      return {
        ok: false,
        message: `Model "${key}" hasScaler mismatch: ml has ${m.hasScaler}, frontend has ${f.hasScaler}.`,
      };
    }
    const apM = m.artifactsPattern && { perFormat: m.artifactsPattern.perFormat, legacy: m.artifactsPattern.legacy };
    const apF = f.artifactsPattern && { perFormat: f.artifactsPattern.perFormat, legacy: f.artifactsPattern.legacy };
    if (JSON.stringify(apM) !== JSON.stringify(apF)) {
      return {
        ok: false,
        message: `Model "${key}" artifactsPattern mismatch. Update frontend defaultModelFeatures.ts to match ml-service app/model_metadata.py _STATIC.`,
      };
    }
  }
  return { ok: true };
}

function main() {
  let goOut;
  let mlOut;
  let frontOut;

  // 1) go-app canonical formats
  const goResult = run(path.join(ROOT, 'go-app'), 'go', ['run', './cmd/print_canonical']);
  if (goResult.status !== 0) {
    console.error('[check-frontend-backend-sync] go-app print_canonical failed:', goResult.stderr || goResult.stdout);
    process.exit(1);
  }
  try {
    goOut = JSON.parse(goResult.stdout.trim());
  } catch (e) {
    console.error('[check-frontend-backend-sync] Failed to parse go-app JSON:', e.message);
    process.exit(1);
  }

  // 2) ml-service model metadata (only model keys, no model_modes)
  const mlCode = `
import json
from app.model_metadata import get_model_metadata
d = get_model_metadata()
out = {k: d[k] for k in ("batting", "bowling", "fielding", "extras", "win", "combination_meta") if k in d}
print(json.dumps(out))
`;
  const mlResult = run(path.join(ROOT, 'ml-service'), 'python', ['-c', mlCode], { PYTHONPATH: '.' });
  if (mlResult.status !== 0) {
    console.error('[check-frontend-backend-sync] ml-service model_metadata failed:', mlResult.stderr || mlResult.stdout);
    process.exit(1);
  }
  try {
    mlOut = JSON.parse(mlResult.stdout.trim());
  } catch (e) {
    console.error('[check-frontend-backend-sync] Failed to parse ml-service JSON:', e.message);
    process.exit(1);
  }

  // 3) frontend export-canonical (requires node and npm in frontend)
  const frontResult = run(path.join(ROOT, 'frontend'), 'npm', ['run', '-s', 'export-canonical']);
  if (frontResult.status !== 0) {
    console.error('[check-frontend-backend-sync] frontend export-canonical failed:', frontResult.stderr || frontResult.stdout);
    process.exit(1);
  }
  try {
    frontOut = JSON.parse(frontResult.stdout.trim());
  } catch (e) {
    console.error('[check-frontend-backend-sync] Failed to parse frontend JSON:', e.message);
    process.exit(1);
  }

  const formatsResult = compareFormats(goOut.formats || [], frontOut.FORMATS || []);
  if (!formatsResult.ok) {
    console.error('[check-frontend-backend-sync]', formatsResult.message);
    process.exit(1);
  }

  const metaResult = compareModelMetadata(mlOut, frontOut);
  if (!metaResult.ok) {
    console.error('[check-frontend-backend-sync]', metaResult.message);
    process.exit(1);
  }

  console.log('[check-frontend-backend-sync] Formats and model metadata match go-app and ml-service.');
}

main();
