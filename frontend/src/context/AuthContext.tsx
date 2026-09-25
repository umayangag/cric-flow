import React, { createContext, useContext, useState, ReactNode } from 'react';
import { getStoredApiKey, setStoredApiKey, clearStoredApiKey } from '../lib/apiKeyStorage';

interface AuthContextType {
  isAuthenticated: boolean;
  login: (password: string) => Promise<boolean>;
  logout: () => void;
  apiKey: string | null;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const AuthProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [apiKey, setApiKey] = useState<string | null>(getStoredApiKey());

  const login = async (password: string): Promise<boolean> => {
    // In local development, the API key is used as the password.
    // We validate the key by making a request to a protected endpoint.
    if (password) {
      try {
        const baseUrl = import.meta.env.VITE_API_URL || 'http://localhost:8080';
        const res = await fetch(`${baseUrl}/ops/status`, {
          headers: {
            'X-API-Key': password,
          },
        });

        if (res.ok) {
          setStoredApiKey(password);
          setApiKey(password);
          return true;
        }
      } catch (err) {
        console.error('Login validation failed:', err);
      }
    }
    return false;
  };

  const logout = () => {
    clearStoredApiKey();
    setApiKey(null);
  };

  const isAuthenticated = !!apiKey;

  return (
    <AuthContext.Provider value={{ isAuthenticated, login, logout, apiKey }}>
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};
