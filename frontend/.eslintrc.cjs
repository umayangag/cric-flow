/* eslint-env node */
module.exports = {
  root: true,
  env: {
    browser: true,
    es2023: true,
    node: true,
  },
  parser: '@typescript-eslint/parser',
  parserOptions: {
    ecmaVersion: 'latest',
    sourceType: 'module',
  },
  settings: {
    react: { version: 'detect' },
  },
  plugins: ['@typescript-eslint', 'react', 'react-hooks', 'prettier'],
  extends: [
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended',
    'plugin:react/recommended',
    'plugin:react-hooks/recommended',
    'plugin:prettier/recommended',
  ],
  rules: {
    // Keep rules minimal and compatible with Prettier
    // Relax to warning to avoid massive unrelated CI failures during structural cleanup
    'prettier/prettier': 'warn',
    'react/react-in-jsx-scope': 'off',
    '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_' }],
    '@typescript-eslint/no-explicit-any': 'warn',
    // Each distinct specifier into a package is its own Vite pre-bundle entry.
    // Many entries make esbuild code-split the package across shared chunks, and
    // a re-optimization then re-splits them, leaving an open tab with chunk URLs
    // whose files no longer agree — a blank page and `styled_default is not a
    // function`. Barrel imports keep @mui/material at exactly one entry.
    // @mui/icons-material is exempt: it is far too large to barrel-import.
    'no-restricted-imports': [
      'error',
      {
        patterns: [
          {
            group: ['@mui/material/*'],
            message:
              "Import from the @mui/material barrel instead: import { Button } from '@mui/material'. Subpath imports add Vite pre-bundle entries and cause blank-page dep-cache crashes.",
          },
        ],
      },
    ],
  },
  ignorePatterns: ['dist/', 'node_modules/', '**/*.config.*', '**/vite.*'],
};
