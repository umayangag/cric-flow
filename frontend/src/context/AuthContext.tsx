import React, { createContext, useContext, useState, useEffect, ReactNode } from 'react';

interface AuthContextType {
  isAuthenticated: boolean;
  login: (username: string, password: string) => Promise<boolean>;
  logout: () => void;
  apiKey: string | null;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

const LOCAL_STORAGE_KEY = 'cric_info_api_key';

// In a real local dev environment, we'd probably get this from a .env file
// but the requirement says hardcoded for now.
const HARDCODED_API_KEY = 'dev-local-key';

export const AuthProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [apiKey, setApiKey] = useState<string | null>(localStorage.getItem(LOCAL_STORAGE_KEY));

  const login = async (username: string, password: string): Promise<boolean> => {
    // Simple hardcoded check as requested
    if (username === 'admin' && password === 'admin') {
      localStorage.setItem(LOCAL_STORAGE_KEY, HARDCODED_API_KEY);
      setApiKey(HARDCODED_API_KEY);
      return true;
    }
    return false;
  };

  const logout = () => {
    localStorage.removeItem(LOCAL_STORAGE_KEY);
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
