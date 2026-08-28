import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../api';

/** Below this many characters a venue search matches most of the table; don't ask. */
const MIN_QUERY = 3;

/** How long to wait after the last keystroke before searching. */
const DEBOUNCE_MS = 300;

/**
 * Venue autocomplete: debounced, and quiet about its own failures.
 *
 * A failed venue lookup offers no suggestions, which is what a user sees anyway when
 * there are none. Turning it into an error banner would interrupt a prediction over
 * an optional field.
 */
export function useVenueSearch() {
  const [options, setOptions] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Only the newest query may write results: a slow "Lor" must not land on top of
  // "Lord's" and replace the suggestions for what the user has actually typed.
  const latest = useRef(0);

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  const search = useCallback((query: string) => {
    const trimmed = query.trim();
    if (timer.current) clearTimeout(timer.current);
    if (trimmed.length < MIN_QUERY) {
      latest.current += 1;
      setOptions([]);
      setLoading(false);
      return;
    }
    timer.current = setTimeout(() => {
      const call = ++latest.current;
      setLoading(true);
      api
        .searchVenues(trimmed)
        .then((list) => call === latest.current && setOptions(list))
        .catch(() => call === latest.current && setOptions([]))
        .finally(() => call === latest.current && setLoading(false));
    }, DEBOUNCE_MS);
  }, []);

  return { options, loading, search };
}
