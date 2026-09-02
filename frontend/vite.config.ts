/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  optimizeDeps: {
    // Every distinct specifier into a package is its own pre-bundle entry. Many
    // entries force esbuild to code-split the package across shared chunks, and a
    // re-optimization then re-splits them all — which is how an open tab ends up
    // holding chunk URLs whose files no longer agree (`styled_default is not a
    // function`, a blank page). Source imports go through the `@mui/material`
    // barrel for exactly this reason; keep them that way and this list short.
    include: [
      '@emotion/react',
      '@emotion/styled',
      // CJS deps that need interop to expose the exports MUI reaches for
      // (prop-types default, react-is ForwardRef).
      'prop-types',
      'react-is',
      '@mui/material',
    ],
  },
  build: {
    chunkSizeWarningLimit: 800,
  },
  server: {
    port: 5173,
    host: true,
    // Fail loudly instead of falling back to the next free port: a second server
    // on 5174 leaves the browser talking to the original one, whose dep cache
    // `dev:clean` has just deleted — which presents as a blank page.
    strictPort: true,
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['src/setupTests.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: [
        '**/*.test.{ts,tsx}',
        '**/*.spec.{ts,tsx}',
        '**/__tests__/**',
        '**/tests/**',
        '**/*.d.ts',
        'src/types/papaparse.d.ts',
      ],
      // Vitest 2 reads the gate from coverage.thresholds. Set at the top level these
      // keys are silently ignored, so the numbers below had never failed a run.
      // Set to the measured figures rounded down; raise them, never lower them.
      thresholds: {
        lines: 77,
        functions: 74,
        statements: 77,
        branches: 78,
      },
    },
  },
});
