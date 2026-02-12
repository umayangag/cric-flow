import React, { createContext, useContext, useState, ReactNode } from 'react';

interface AuthContextType {
  isAuthenticated: boolean;
  login: (password: string) => Promise<boolean>;
  logout: () => void;
  apiKey: string | null;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

const STORAGE_KEY = 'cric_info_api_key';


export const AuthProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [apiKey, setApiKey] = useState<string | null>(sessionStorage.getItem(STORAGE_KEY));

  const login = async (password: string): Promise<boolean> => {
    // In local development, the API key is used as the password.
    // This allows users to set their own API key via the login form.
    if (password) {
      sessionStorage.setItem(STORAGE_KEY, password);
      setApiKey(password);
      return true;
    }
    return false;
  };

  const logout = () => {
    sessionStorage.removeItem(STORAGE_KEY);
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
