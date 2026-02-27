/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    host: true,
  },
  test: {
    globals: true,
    environment: 'jsdom',
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
      lines: 50,
      functions: 50,
      statements: 50,
      branches: 50,
    },
  },
});
