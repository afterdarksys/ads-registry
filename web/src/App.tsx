import { BrowserRouter, Routes, Route, Navigate, useNavigate } from 'react-router-dom';
import { AuthProvider, useAuth } from './contexts/AuthContext';
import { ThemeProvider } from './contexts/ThemeContext';
import Layout from './components/Layout';
import Dashboard from './pages/Dashboard';
import Users from './pages/Users';
import Repositories from './pages/Repositories';
import Policies from './pages/Policies';
import Scripts from './pages/Scripts';
import Settings from './pages/Settings';
import Upstreams from './pages/Upstreams';
import Vulnerabilities from './pages/Vulnerabilities';
import Login from './pages/Login';
import type { ReactNode } from 'react';

// Protected route wrapper that redirects to login if not authenticated
function ProtectedRoute({ children }: { children: ReactNode }) {
  const { isAuthenticated, isLoading } = useAuth();

  if (isLoading) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center">
        <div className="text-center">
          <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-primary"></div>
          <p className="mt-4 text-muted-foreground">Loading...</p>
        </div>
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}

// Login route wrapper that redirects to dashboard if already authenticated
function LoginRoute() {
  const { isAuthenticated } = useAuth();
  const navigate = useNavigate();

  const handleLoginSuccess = () => {
    navigate('/', { replace: true });
  };

  if (isAuthenticated) {
    return <Navigate to="/" replace />;
  }

  return <Login onLoginSuccess={handleLoginSuccess} />;
}

export default function App() {
  return (
    <BrowserRouter>
      <ThemeProvider>
        <AuthProvider>
          <Routes>
            <Route path="/login" element={<LoginRoute />} />
            <Route path="/" element={<ProtectedRoute><Layout /></ProtectedRoute>}>
              <Route index element={<Dashboard />} />
              <Route path="users" element={<Users />} />
              <Route path="repositories" element={<Repositories />} />
              <Route path="upstreams" element={<Upstreams />} />
              <Route path="vulnerabilities" element={<Vulnerabilities />} />
              <Route path="policies" element={<Policies />} />
              <Route path="scripts" element={<Scripts />} />
              <Route path="settings" element={<Settings />} />
            </Route>
          </Routes>
        </AuthProvider>
      </ThemeProvider>
    </BrowserRouter>
  );
}
