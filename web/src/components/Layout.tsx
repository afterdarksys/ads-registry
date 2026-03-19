import { Link, Outlet, useLocation } from 'react-router-dom';
import { LayoutDashboard, Users, Shield, Server, Settings, FileTerminal, Cloud, AlertTriangle, LogOut, ChevronDown, Moon, Sun } from 'lucide-react';
import { useAuth } from '../contexts/AuthContext';
import { useTheme } from '../contexts/ThemeContext';
import { useState } from 'react';

const navigation = [
  { name: 'Dashboard', href: '/', icon: LayoutDashboard },
  { name: 'Users & Groups', href: '/users', icon: Users },
  { name: 'Repositories', href: '/repositories', icon: Server },
  { name: 'Upstreams', href: '/upstreams', icon: Cloud },
  { name: 'Vulnerabilities', href: '/vulnerabilities', icon: AlertTriangle },
  { name: 'Security Policies', href: '/policies', icon: Shield },
  { name: 'Automation Scripts', href: '/scripts', icon: FileTerminal },
  { name: 'Settings', href: '/settings', icon: Settings },
];

export default function Layout() {
  const location = useLocation();
  const { user, logout } = useAuth();
  const { theme, toggleTheme } = useTheme();
  const [showUserMenu, setShowUserMenu] = useState(false);

  const handleLogout = async () => {
    await logout();
    // The AuthContext will clear localStorage and redirect happens via ProtectedRoute
  };

  const getUserInitials = (username: string) => {
    return username.charAt(0).toUpperCase();
  };

  return (
    <div className="flex h-screen bg-muted/40 dark:bg-background">
      {/* Sidebar */}
      <div className="w-64 bg-card border-r border-border shadow-sm flex flex-col">
        <div className="h-16 flex items-center px-6 border-b border-border">
          <Server className="h-6 w-6 text-primary mr-3" />
          <h1 className="text-lg font-bold bg-clip-text text-transparent bg-gradient-to-r from-primary to-primary/60">
            ADS Registry
          </h1>
        </div>
        <nav className="flex-1 px-4 py-6 space-y-1 overflow-y-auto">
          {navigation.map((item) => {
            const isActive = location.pathname === item.href;
            return (
              <Link
                key={item.name}
                to={item.href}
                className={`flex items-center px-3 py-2.5 rounded-md text-sm transition-all duration-200 ${
                  isActive
                    ? 'bg-primary/10 text-primary font-medium'
                    : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground'
                }`}
              >
                <item.icon
                  className={`flex-shrink-0 h-5 w-5 mr-3 transition-colors ${
                    isActive ? 'text-primary' : 'text-muted-foreground group-hover:text-foreground'
                  }`}
                />
                {item.name}
              </Link>
            );
          })}
        </nav>
        <div className="p-4 border-t border-border mt-auto">
          <div className="flex items-center text-sm text-muted-foreground">
            <div className="w-2 h-2 rounded-full bg-green-500 mr-2"></div>
            System Online
          </div>
        </div>
      </div>

      {/* Main content */}
      <div className="flex-1 flex flex-col overflow-hidden">
        <header className="h-16 bg-card border-b border-border shadow-sm flex items-center justify-between px-8">
          <h2 className="text-lg font-semibold text-foreground">
            {navigation.find((item) => item.href === location.pathname)?.name || 'Admin Console'}
          </h2>
          <div className="flex items-center gap-3">
            {/* Dark mode toggle */}
            <button
              onClick={toggleTheme}
              className="p-2 rounded-lg hover:bg-muted/50 transition-colors"
              title={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
            >
              {theme === 'dark' ? (
                <Sun className="w-5 h-5 text-yellow-500" />
              ) : (
                <Moon className="w-5 h-5 text-slate-700" />
              )}
            </button>

            <div className="relative">
            <button
              onClick={() => setShowUserMenu(!showUserMenu)}
              className="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-muted/50 transition-colors"
            >
              <div className="text-right">
                <div className="text-sm font-medium">{user?.username || 'User'}</div>
                <div className="text-xs text-muted-foreground">
                  {user?.is_admin ? (
                    <span className="inline-flex items-center gap-1">
                      <Shield className="w-3 h-3" />
                      Administrator
                    </span>
                  ) : (
                    `${user?.namespaces.length || 0} namespace${user?.namespaces.length !== 1 ? 's' : ''}`
                  )}
                </div>
              </div>
              <div className="w-9 h-9 rounded-full bg-primary/20 flex items-center justify-center text-primary font-bold">
                {getUserInitials(user?.username || 'U')}
              </div>
              <ChevronDown className={`w-4 h-4 text-muted-foreground transition-transform ${showUserMenu ? 'rotate-180' : ''}`} />
            </button>

            {/* User dropdown menu */}
            {showUserMenu && (
              <>
                <div
                  className="fixed inset-0 z-10"
                  onClick={() => setShowUserMenu(false)}
                ></div>
                <div className="absolute right-0 mt-2 w-64 bg-card border border-border rounded-lg shadow-lg z-20 overflow-hidden">
                  <div className="p-4 border-b border-border">
                    <div className="font-medium">{user?.username}</div>
                    <div className="text-sm text-muted-foreground mt-1">
                      {user?.is_admin ? 'Full system access' : 'Limited access'}
                    </div>
                    {!user?.is_admin && user?.namespaces && user.namespaces.length > 0 && (
                      <div className="mt-2 pt-2 border-t border-border">
                        <div className="text-xs font-medium text-muted-foreground mb-1">Your namespaces:</div>
                        <div className="flex flex-wrap gap-1">
                          {user.namespaces.slice(0, 5).map((ns) => (
                            <span key={ns} className="text-xs px-2 py-0.5 bg-muted rounded">
                              {ns}
                            </span>
                          ))}
                          {user.namespaces.length > 5 && (
                            <span className="text-xs text-muted-foreground">
                              +{user.namespaces.length - 5} more
                            </span>
                          )}
                        </div>
                      </div>
                    )}
                  </div>
                  <button
                    onClick={handleLogout}
                    className="w-full flex items-center gap-3 px-4 py-3 text-sm hover:bg-muted/50 transition-colors text-red-500"
                  >
                    <LogOut className="w-4 h-4" />
                    Sign Out
                  </button>
                </div>
              </>
            )}
            </div>
          </div>
        </header>
        <main className="flex-1 overflow-auto p-8 bg-muted/20">
          <div className="max-w-6xl mx-auto">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  );
}
