import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { AuthProvider } from '../context/AuthContext';
import Login from './Login';

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

const renderLogin = (initialPath = '/login') => {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/" element={<div data-testid="home">Home</div>} />
          <Route path="/ops" element={<div data-testid="ops">Ops</div>} />
        </Routes>
      </AuthProvider>
    </MemoryRouter>,
  );
};

describe('Login', () => {
  const originalFetch = globalThis.fetch;
  beforeEach(() => {
    globalThis.fetch = vi.fn();
    vi.stubGlobal('sessionStorage', createStorage());
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.unstubAllGlobals();
  });

  it('renders Admin Login title and API key field', () => {
    renderLogin();
    expect(screen.getByRole('heading', { name: /admin login/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/api key/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /login/i })).toBeInTheDocument();
  });

  it('shows error when login returns false', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ ok: false });
    const user = userEvent.setup();
    renderLogin();
    await user.type(screen.getByLabelText(/api key/i), 'wrong');
    await user.click(screen.getByRole('button', { name: /login/i }));
    expect(
      await screen.findByText(/invalid api key or server not configured/i),
    ).toBeInTheDocument();
  });

  it('navigates to from path when login succeeds', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ ok: true });
    const user = userEvent.setup();
    render(
      <MemoryRouter
        initialEntries={[{ pathname: '/login', state: { from: { pathname: '/ops' } } }]}
      >
        <AuthProvider>
          <Routes>
            <Route path="/login" element={<Login />} />
            <Route path="/ops" element={<div data-testid="ops">Ops</div>} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>,
    );
    await user.type(screen.getByLabelText(/api key/i), 'valid-key');
    await user.click(screen.getByRole('button', { name: /login/i }));
    expect(await screen.findByTestId('ops')).toBeInTheDocument();
  });
});
