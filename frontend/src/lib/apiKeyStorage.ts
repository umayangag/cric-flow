/**
 * Where the admin API key lives in the browser (OPS-03).
 *
 * `localStorage` kept the key on the origin indefinitely — until someone cleared site
 * data by hand, a stolen or leaked key (an XSS payload, a shared machine, a saved
 * profile) stayed valid forever. `sessionStorage` is the fix that costs the least: it
 * still survives an accidental reload of the page the operator is watching (a pipeline
 * run's progress stream can run for a while), but it does not survive closing the tab
 * or the browser, and it is never shared with a new tab — so the key must be re-entered
 * whenever the ops console is opened fresh, which is the user-visible cost of no longer
 * leaving it lying around.
 *
 * `AuthContext` and `api.ts` both need this value — the former to know whether the
 * operator is logged in, the latter to attach it to a request — so it is read and
 * written in exactly one place rather than each reaching into browser storage with its
 * own copy of the key name.
 */

const API_KEY_STORAGE_KEY = 'cric_info_api_key';

export function getStoredApiKey(): string | null {
  return window.sessionStorage.getItem(API_KEY_STORAGE_KEY);
}

export function setStoredApiKey(key: string): void {
  window.sessionStorage.setItem(API_KEY_STORAGE_KEY, key);
}

export function clearStoredApiKey(): void {
  window.sessionStorage.removeItem(API_KEY_STORAGE_KEY);
}
