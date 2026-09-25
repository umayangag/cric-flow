import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AuthProvider, useAuth } from './AuthContext';

// Component that uses useAuth for testing
// eslint-disable-next-line react/prop-types -- test helper; props are typed via TS
const Consumer: React.FC<{ onLogout?: () => void }> = ({ onLogout }) => {
  const { isAuthenticated, login, logout, apiKey } = useAuth();
  return (
    <div>
      <span data-testid="authenticated">{String(isAuthenticated)}</span>
      <span data-testid="apikey">{apiKey ?? 'null'}</span>
      <button type="button" onClick={() => login('test-key')}>
        Do login
      </button>
      <button
        type="button"
        onClick={() => {
          logout();
          onLogout?.();
        }}
      >
        Logout
      </button>
    </div>
  );
};

const createStorage = (): Storage => {
  const map = new Map<string, string>();
  return {
    getItem: (key: string) => map.get(key) ?? null,
    setItem: (key: string, value: string) => map.set(key, value),
    removeItem: (key: string) => map.delete(key),
    clear: () => map.clear(),
    get length() {
      return map.size;
    },
    key: (i: number) => Array.from(map.keys())[i] ?? null,
  };
};

describe('AuthContext', () => {
  const originalFetch = globalThis.fetch;
  let storage: Storage;
  beforeEach(() => {
    globalThis.fetch = vi.fn();
    storage = createStorage();
    vi.stubGlobal('sessionStorage', storage);
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.unstubAllGlobals();
  });

  it('useAuth throws when used outside AuthProvider', () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const Throwing = () => {
      useAuth();
      return null;
    };
    expect(() => render(<Throwing />)).toThrow('useAuth must be used within an AuthProvider');
    consoleErrorSpy.mockRestore();
  });

  it('provides isAuthenticated false and null apiKey when no key in storage', () => {
    render(
      <AuthProvider>
        <Consumer />
      </AuthProvider>,
    );
    expect(screen.getByTestId('authenticated')).toHaveTextContent('false');
    expect(screen.getByTestId('apikey')).toHaveTextContent('null');
  });

  it('provides isAuthenticated true when key is in sessionStorage', () => {
    storage.setItem('cric_info_api_key', 'stored-key');
    render(
      <AuthProvider>
        <Consumer />
      </AuthProvider>,
    );
    expect(screen.getByTestId('authenticated')).toHaveTextContent('true');
    expect(screen.getByTestId('apikey')).toHaveTextContent('stored-key');
  });

  it('login succeeds when fetch returns ok and updates state', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ ok: true });
    const user = userEvent.setup();
    render(
      <AuthProvider>
        <Consumer />
      </AuthProvider>,
    );
    await user.click(screen.getByText('Do login'));
    await act(async () => {});
    expect(storage.getItem('cric_info_api_key')).toBe('test-key');
    expect(screen.getByTestId('authenticated')).toHaveTextContent('true');
    expect(screen.getByTestId('apikey')).toHaveTextContent('test-key');
  });

  it('login returns false when fetch returns not ok', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ ok: false });
    const user = userEvent.setup();
    render(
      <AuthProvider>
        <Consumer />
      </AuthProvider>,
    );
    await user.click(screen.getByText('Do login'));
    await act(async () => {});
    expect(storage.getItem('cric_info_api_key')).toBeNull();
    expect(screen.getByTestId('authenticated')).toHaveTextContent('false');
  });

  it('logout clears storage and apiKey', async () => {
    storage.setItem('cric_info_api_key', 'old-key');
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ ok: true });
    const user = userEvent.setup();
    render(
      <AuthProvider>
        <Consumer />
      </AuthProvider>,
    );
    await user.click(screen.getByText('Logout'));
    expect(storage.getItem('cric_info_api_key')).toBeNull();
    expect(screen.getByTestId('authenticated')).toHaveTextContent('false');
  });
});
