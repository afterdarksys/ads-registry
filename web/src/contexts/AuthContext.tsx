import { createContext, useContext, useState, useEffect } from 'react';
import type { ReactNode } from 'react';

export interface UserInfo {
  username: string;
  scopes: string[];
  is_admin: boolean;
  namespaces: string[];
}

interface AuthContextType {
  user: UserInfo | null;
  token: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (token: string, user: UserInfo) => void;
  logout: () => void;
  refreshUser: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<UserInfo | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  // Initialize auth state from localStorage
  useEffect(() => {
    const initAuth = async () => {
      const storedToken = localStorage.getItem('authToken');
      const storedUser = localStorage.getItem('authUser');
      const storedExpiry = localStorage.getItem('authExpiry');

      if (storedToken && storedUser && storedExpiry) {
        // Check if token is expired
        const expiryDate = new Date(storedExpiry);
        if (expiryDate > new Date()) {
          setToken(storedToken);
          setUser(JSON.parse(storedUser));

          // Verify token is still valid by calling /oauth2/me
          try {
            await refreshUserInfo(storedToken);
          } catch (error) {
            console.error('Token validation failed:', error);
            clearAuth();
          }
        } else {
          // Token expired
          clearAuth();
        }
      }

      setIsLoading(false);
    };

    initAuth();
  }, []);

  const refreshUserInfo = async (authToken: string) => {
    const response = await fetch('/oauth2/me', {
      headers: {
        'Authorization': `Bearer ${authToken}`,
      },
    });

    if (!response.ok) {
      throw new Error('Failed to fetch user info');
    }

    const userData = await response.json();
    setUser(userData);
    localStorage.setItem('authUser', JSON.stringify(userData));
  };

  const login = (newToken: string, newUser: UserInfo) => {
    setToken(newToken);
    setUser(newUser);
    localStorage.setItem('authToken', newToken);
    localStorage.setItem('authUser', JSON.stringify(newUser));
  };

  const logout = async () => {
    // Call logout endpoint
    if (token) {
      try {
        await fetch('/oauth2/logout', {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${token}`,
          },
        });
      } catch (error) {
        console.error('Logout request failed:', error);
      }
    }

    clearAuth();
  };

  const clearAuth = () => {
    setToken(null);
    setUser(null);
    localStorage.removeItem('authToken');
    localStorage.removeItem('authUser');
    localStorage.removeItem('authExpiry');
  };

  const refreshUser = async () => {
    if (token) {
      await refreshUserInfo(token);
    }
  };

  const value: AuthContextType = {
    user,
    token,
    isAuthenticated: !!user && !!token,
    isLoading,
    login,
    logout,
    refreshUser,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}
