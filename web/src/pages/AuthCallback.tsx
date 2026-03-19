import { useEffect } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';

export default function AuthCallback() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { login } = useAuth();

  useEffect(() => {
    const token = searchParams.get('token');
    const expiresAt = searchParams.get('expires_at');
    const username = searchParams.get('username');
    const isAdmin = searchParams.get('is_admin') === 'true';

    if (token && expiresAt && username) {
      // Store the authentication data
      const user = {
        username,
        scopes: isAdmin ? ['*'] : [],
        is_admin: isAdmin,
        namespaces: [],
      };

      localStorage.setItem('authToken', token);
      localStorage.setItem('authUser', JSON.stringify(user));
      localStorage.setItem('authExpiry', expiresAt);

      // Update auth context
      login(token, user);

      // Redirect to dashboard
      navigate('/', { replace: true });
    } else {
      // Missing parameters, redirect to login
      navigate('/login', { replace: true });
    }
  }, [searchParams, login, navigate]);

  return (
    <div className="min-h-screen bg-background flex items-center justify-center">
      <div className="text-center">
        <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-primary"></div>
        <p className="mt-4 text-muted-foreground">Completing sign in...</p>
      </div>
    </div>
  );
}
