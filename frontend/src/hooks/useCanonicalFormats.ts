import { useState, useEffect } from 'react';
import { api } from '../api';

let cachedFormats: string[] | null = null;
let fetchPromise: Promise<string[]> | null = null;

function getCanonicalFormatsCached(): Promise<string[]> {
  if (cachedFormats !== null) return Promise.resolve(cachedFormats);
  if (fetchPromise === null) {
    fetchPromise = api.getCanonicalFormats().then((f) => {
      cachedFormats = f;
      return f;
    });
  }
  return fetchPromise;
}

export type FormatCode = string;

export interface UseCanonicalFormatsReturn {
  formats: FormatCode[];
  loading: boolean;
  error: string | null;
}

/** Fetches canonical format codes (TEST, ODI, T20, T20I) from go-app. Cached so multiple callers share one request. */
export function useCanonicalFormats(): UseCanonicalFormatsReturn {
  const [formats, setFormats] = useState<FormatCode[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(null);
    getCanonicalFormatsCached()
      .then((f) => {
        if (active) setFormats(f);
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : String(e));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  return { formats, loading, error };
}
