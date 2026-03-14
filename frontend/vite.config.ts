/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  optimizeDeps: {
    // Exclude MUI from pre-bundling to avoid createTheme_default ESM/CJS interop errors in dev
    exclude: ['@mui/material', '@mui/material/styles'],
    include: ['@emotion/react', '@emotion/styled'],
  },
  build: {
    chunkSizeWarningLimit: 800,
  },
  server: {
    port: 5173,
    host: true,
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
      // Soft gate: fail if coverage drops below this (raise over time)
      lines: 59,
      functions: 57,
      statements: 59,
      branches: 70,
    },
  },
});
