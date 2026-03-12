import { Link, Outlet, useLocation } from 'react-router-dom';
import { LayoutDashboard, Users, Shield, Server, Settings, FileTerminal } from 'lucide-react';

const navigation = [
  { name: 'Dashboard', href: '/', icon: LayoutDashboard },
  { name: 'Users & Groups', href: '/users', icon: Users },
  { name: 'Repositories', href: '/repositories', icon: Server },
  { name: 'Security Policies', href: '/policies', icon: Shield },
  { name: 'Automation Scripts', href: '/scripts', icon: FileTerminal },
  { name: 'Settings', href: '/settings', icon: Settings },
];

export default function Layout() {
  const location = useLocation();

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
          <div className="flex items-center gap-4">
            <span className="text-sm font-medium">Administrator</span>
            <div className="w-8 h-8 rounded-full bg-primary/20 flex items-center justify-center text-primary font-bold">
              A
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
