/**
 * Export frontend canonical config (FORMATS, MODEL_KEYS, DEFAULT_MODEL_FEATURES) as JSON.
 * Used by scripts/check-frontend-backend-sync.mjs to compare with go-app and ml-service.
 * Run from repo root: cd frontend && npx tsx scripts/export-canonical.ts
 */
import { DEFAULT_MODEL_FEATURES, MODEL_KEYS } from '../src/constants/defaultModelFeatures';
import { FORMATS } from '../src/utils/opsStatusHelpers';

const out = {
  FORMATS: [...FORMATS],
  MODEL_KEYS: [...MODEL_KEYS],
  DEFAULT_MODEL_FEATURES,
};
console.log(JSON.stringify(out));
