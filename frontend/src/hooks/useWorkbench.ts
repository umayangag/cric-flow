import { api } from '../api';
import { useAsync } from './useAsync';
import type { ApiError } from '../lib/apiError';
import type { XiStatusResponse } from '../types';
import { readFreshness, UNKNOWN_FRESHNESS } from '../utils/opsStatusHelpers';
import type { OpsFreshness } from '../utils/opsStatusHelpers';

export interface UseWorkbenchReturn {
  /** The loaded run and its manifest, from GET /api/ml/xi-status. Null until loaded or on error. */
  runStatus: XiStatusResponse | null;
  runStatusLoading: boolean;
  runStatusError: ApiError | null;
  /**
   * The one freshness object (P2-1). The loaded-run card shows the date its ratings run
   * through, and that date is the verdict's — read off /ops/status like every other
   * freshness surface, not worked out again from the run status beside it.
   */
  freshness: OpsFreshness;
}

/**
 * The Workbench's state: what each loaded model *is*.
 *
 * How well the models predict is the Evaluation report tab, which reads L4's own
 * measurements. The accuracy trend that used to live here re-scored one match at a time
 * against the batting, bowling and fielding models, and went with them in P-5; the
 * walk-forward registry upload went in F-1 (D-8), because the module that wrote those
 * files went with them too — walk-forward evaluation is L4's job and its numbers are on
 * the Evaluation tab.
 */
export function useWorkbench(): UseWorkbenchReturn {
  // One request: which run is loaded, and what its manifest says. Provenance used to be
  // read from a per-artifact sidecar and the feature list from the win model's metadata;
  // both were inferences about an artifact, and the manifest is the record (H-16).
  const runStatus = useAsync(api.xiStatus, {
    runOnMount: [],
    errorMessage: 'Failed to load the run status',
  });
  const opsStatus = useAsync(api.opsStatus, {
    runOnMount: [],
    errorMessage: 'Failed to load the ops status',
  });

  return {
    runStatus: runStatus.data,
    runStatusLoading: runStatus.loading,
    runStatusError: runStatus.error,
    freshness: opsStatus.data ? readFreshness(opsStatus.data) : UNKNOWN_FRESHNESS,
  };
}
