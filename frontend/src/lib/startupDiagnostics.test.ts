import { describeStartupError } from './startupDiagnostics';

describe('describeStartupError', () => {
  it.each([
    ['styled_default is not a function'],
    ['Failed to fetch dynamically imported module: /src/components/HealthTab.tsx'],
    ['Importing a module script failed.'],
    ['504 (Outdated Optimize Dep)'],
    ["The requested module does not provide an export named 'default'"],
  ])('flags %s as a stale dependency cache', (message) => {
    const diagnosis = describeStartupError(message);

    expect(diagnosis.command).toContain('dev:clean');
    expect(diagnosis.hint).toMatch(/stale Vite dependency cache/i);
  });

  it('gives a generic hint for an unrelated application error', () => {
    const diagnosis = describeStartupError("Cannot read properties of undefined (reading 'map')");

    expect(diagnosis.command).toBeNull();
    expect(diagnosis.hint).toMatch(/browser console/i);
  });
});
