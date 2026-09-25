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
      // The icons barrel, for the same reason: a deep import per icon is both an extra
      // pre-bundle entry and a CJS file, and the dev optimizer hands a CJS default back
      // unwrapped (the icon arrives as {default: Component}, which React rejects).
      '@mui/icons-material',
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
    fs: {
      // The System map imports contracts/system-map.json directly rather than keeping a
      // copy of it under src/. That file is the source of truth CI checks against the
      // code, and a mirrored copy would be exactly the drift the map exists to prevent —
      // so the dev server is allowed to read the repo root. The build resolves it as an
      // ordinary relative import and inlines it.
      allow: ['..'],
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./vitest.setup.ts'],
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
      //
      // The second time the same gate was silently off: a `vitest.config.ts` beside this
      // file took precedence over it, so vitest read its `test` block and never saw these
      // thresholds -- verified by renaming it, which turned three of the four gates red.
      // That file duplicated what is here and has been deleted; the setup file it owned
      // moved to `setupFiles` above.
      //
      // Set to the measured figures rounded down; raise them, never lower them.
      //
      // Re-baselined once, at the vitest 2 -> 5 bump. Vitest 5 makes AST-aware
      // remapping mandatory (the opt-out flag is gone), which redefines these
      // metrics rather than measuring the suite differently well: the same 337
      // tests over the same sources scored 82.82/80.98/77.93/82.82 under vitest 2
      // and 76.61/69.36/76.03/78.75 under vitest 5. The tell is statements and
      // lines, which were one number under the old provider and are now two.
      // A change of ruler, not of test quality -- so the figures below are the
      // new measurement rounded down, and the ratchet resumes from them.
      //
      // Lines 81 -> 82 with P2-4 (the track record): CI run 34145895909 measured
      // 82.31 lines, the same figure as the local run.
      //
      // Lines 82 -> 83, functions 81 -> 82 and statements 80 -> 81 with P3-1 (the
      // Auction tab): CI run 34266207576 measured 81 / 73.92 / 82.76 / 83.23, the same
      // figures as the local run. Branches stay at 73, which is what 73.92 rounds down to.
      //
      // Functions 82 -> 83 with OPS-03 (the API key's storage and attachment rule): the
      // local run measured 81.63 / 74.7 / 83.03 / 83.8 (statements/branches/functions/
      // lines) -- lines, statements and branches round down to their existing floors,
      // functions to one above its old floor.
      thresholds: {
        lines: 83,
        functions: 83,
        statements: 81,
        branches: 74,
      },
    },
  },
});
