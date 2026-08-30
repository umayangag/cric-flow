export interface StartupDiagnosis {
  /** Plain-language explanation of what this class of error usually means. */
  hint: string;
  /** Shell command that resolves it, when there is a known one. */
  command: string | null;
}

/**
 * Vite serves pre-bundled dependency chunks by path and ignores the `?v=` hash.
 * When it re-optimizes mid-session, an open tab can end up holding chunk URLs
 * from the previous run whose files have since been re-split, which surfaces as
 * a missing lazy-init binding (`styled_default is not a function`) or a failed
 * dynamic import. Both are cured by discarding node_modules/.vite.
 */
const STALE_DEP_CACHE_PATTERNS: RegExp[] = [
  /\b\w+_default is not a function\b/,
  /Failed to fetch dynamically imported module/i,
  /Importing a module script failed/i,
  /outdated optimize dep/i,
  /does not provide an export named/i,
];

export function describeStartupError(message: string): StartupDiagnosis {
  const isStaleDepCache = STALE_DEP_CACHE_PATTERNS.some((pattern) => pattern.test(message));

  if (isStaleDepCache) {
    return {
      hint:
        'This is a stale Vite dependency cache, not a bug in the page. Vite re-optimized its ' +
        'pre-bundled dependencies while this tab was open, so the chunks no longer agree. ' +
        'Restart the dev server with a clean cache:',
      command: 'npm run dev:clean   # from frontend/',
    };
  }

  return {
    hint: 'Check the browser console for the full stack trace.',
    command: null,
  };
}
