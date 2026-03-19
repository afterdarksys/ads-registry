import { useState, useEffect } from 'react';
import { Server, HardDrive, AlertTriangle, Shield } from 'lucide-react';

export default function Dashboard() {
  const [stats, setStats] = useState<any>({
    total_repos: 0,
    storage_used: '0 B',
    critical_vulns: 0,
    policy_blocks: 0
  });

  useEffect(() => {
    fetch('/api/v1/management/stats')
      .then(res => res.json())
      .then(data => setStats(data))
      .catch(console.error);
  }, []);

  const cards = [
    {
      title: 'Total Repositories',
      value: stats.total_repos,
      icon: Server,
      color: 'text-blue-500'
    },
    {
      title: 'Storage Used',
      value: stats.storage_used,
      icon: HardDrive,
      color: 'text-green-500'
    },
    {
      title: 'Critical Vulnerabilities',
      value: stats.critical_vulns,
      icon: AlertTriangle,
      color: stats.critical_vulns > 0 ? 'text-red-500' : 'text-muted-foreground'
    },
    {
      title: 'Policy Blocks',
      value: stats.policy_blocks,
      icon: Shield,
      color: 'text-purple-500'
    }
  ];

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Dashboard</h1>
        <p className="text-muted-foreground">System overview and statistics.</p>
      </div>

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        {cards.map((card) => {
          const Icon = card.icon;
          return (
            <div key={card.title} className="bg-card border border-border rounded-xl p-6 shadow-sm hover:shadow-md transition-shadow">
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-sm font-medium text-muted-foreground">{card.title}</h3>
                <Icon className={`w-5 h-5 ${card.color}`} />
              </div>
              <div className="text-3xl font-bold tracking-tight">{card.value}</div>
            </div>
          );
        })}
      </div>

      <div className="bg-card border border-border rounded-xl p-6 shadow-sm">
        <h3 className="text-lg font-semibold mb-3">Quick Links</h3>
        <div className="grid gap-3 md:grid-cols-3">
          <a href="#/repositories" className="block p-4 bg-muted/50 hover:bg-muted rounded-lg transition-colors">
            <Server className="w-5 h-5 mb-2 text-primary" />
            <div className="font-medium">Repositories</div>
            <div className="text-xs text-muted-foreground">Manage images and tags</div>
          </a>
          <a href="#/vulnerabilities" className="block p-4 bg-muted/50 hover:bg-muted rounded-lg transition-colors">
            <AlertTriangle className="w-5 h-5 mb-2 text-orange-500" />
            <div className="font-medium">Vulnerabilities</div>
            <div className="text-xs text-muted-foreground">Security scan results</div>
          </a>
          <a href="#/upstreams" className="block p-4 bg-muted/50 hover:bg-muted rounded-lg transition-colors">
            <HardDrive className="w-5 h-5 mb-2 text-green-500" />
            <div className="font-medium">Upstreams</div>
            <div className="text-xs text-muted-foreground">Proxy registries</div>
          </a>
        </div>
      </div>
    </div>
  );
}
