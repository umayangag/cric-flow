import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom',
    // Enable global test APIs like describe/it/expect for tests that
    // don't import them explicitly (e.g., files under tests/components)
    globals: true,
    setupFiles: ['./vitest.setup.ts'],
  },
});
