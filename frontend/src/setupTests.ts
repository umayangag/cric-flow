// Global Vitest setup: ensure localStorage is jsdom-backed, not Node's webstorage.
if (typeof window !== 'undefined' && window.localStorage) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (globalThis as any).localStorage = window.localStorage;
}
