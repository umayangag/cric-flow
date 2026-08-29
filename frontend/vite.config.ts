/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  optimizeDeps: {
    // Pre-bundle React/MUI CJS deps so they provide expected ESM exports (prop-types default, react-is ForwardRef, etc.)
    include: [
      '@emotion/react',
      '@emotion/styled',
      'prop-types',
      'react-is',
      '@mui/material',
      '@mui/material/styles',
    ],
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
      lines: 64,
      functions: 63,
      statements: 64,
      branches: 74,
    },
  },
});
